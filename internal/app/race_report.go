package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type RaceReport struct {
	Revision    int      `json:"revision"`
	Sections    []string `json:"sections"`
	Training    string   `json:"training"`
	Preparation string   `json:"preparation"`
	Experience  string   `json:"experience"`
	Reflections string   `json:"reflections"`
}

func defaultRaceReport() RaceReport {
	return RaceReport{Sections: []string{"facts", "goals", "results", "checkpoints", "halfway", "narrative"}}
}
func validateRaceReport(draft RaceReport) error {
	if draft.Sections == nil {
		return fmt.Errorf("%w: report sections must be an array", ErrRaceInvalid)
	}
	for _, text := range []string{draft.Training, draft.Preparation, draft.Experience, draft.Reflections} {
		if len([]rune(text)) > 20000 {
			return fmt.Errorf("%w: each narrative section is limited to 20,000 characters", ErrRaceInvalid)
		}
	}
	allowed := []string{"facts", "goals", "results", "checkpoints", "laps", "halfway", "buildup", "weather", "narrative"}
	if len(draft.Sections) > len(allowed) {
		return ErrRaceInvalid
	}
	for _, section := range draft.Sections {
		if !slices.Contains(allowed, section) {
			return ErrRaceInvalid
		}
	}
	return nil
}
func (s *Store) RaceReport(ctx context.Context, id string) (RaceReport, error) {
	if _, err := s.GetRace(ctx, id); err != nil {
		return RaceReport{}, err
	}
	result := defaultRaceReport()
	var data []byte
	var revision int
	err := s.db.QueryRow(ctx, `select data,revision from race_reports where race_id=$1 and user_id=$2`, id, scopedUserID(ctx)).Scan(&data, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	result.Revision = revision
	return result, err
}
func (s *Store) SaveRaceReport(ctx context.Context, id string, input RaceReport) (RaceReport, error) {
	if err := validateRaceReport(input); err != nil {
		return input, err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return input, err
	}
	err = s.db.QueryRow(ctx, `insert into race_reports(race_id,user_id,data) select id,user_id,$3 from races where id=$1 and user_id=$2 on conflict(race_id) do update set data=excluded.data,revision=race_reports.revision+1,updated_at=now() where race_reports.user_id=$2 and race_reports.revision=$4 returning revision`, id, scopedUserID(ctx), data, input.Revision).Scan(&input.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return input, ErrRaceConflict
	}
	return input, err
}
func raceDuration(ms int64) string {
	sign := ""
	if ms < 0 {
		sign = "-"
		ms = -ms
	}
	seconds := ms / 1000
	fraction := ""
	if ms%1000 != 0 {
		fraction = strings.TrimRight(fmt.Sprintf(".%03d", ms%1000), "0")
	}
	if seconds >= 3600 {
		return fmt.Sprintf("%s%d:%02d:%02d%s", sign, seconds/3600, (seconds/60)%60, seconds%60, fraction)
	}
	return fmt.Sprintf("%s%d:%02d%s", sign, seconds/60, seconds%60, fraction)
}
func raceMarkdownCell(text string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "|", "\\|", "\r", " ", "\n", " ", "<", "&lt;", ">", "&gt;", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`")
	return replacer.Replace(text)
}
func raceReportMarkdown(detail RaceDetail, draft RaceReport) string {
	var b strings.Builder
	r := detail.Race
	fmt.Fprintf(&b, "# %s\n\n", raceMarkdownCell(r.Name))
	has := func(s string) bool { return slices.Contains(draft.Sections, s) }
	if has("facts") {
		fmt.Fprintf(&b, "## Race information\n\n")
		if r.Date != "" {
			fmt.Fprintf(&b, "- Date: %s\n", r.Date)
		}
		if r.DistanceM != nil {
			fmt.Fprintf(&b, "- Distance: %g km\n", *r.DistanceM/1000)
		}
		fmt.Fprintf(&b, "- Discipline: %s\n- Event kind: %s\n- Outcome: %s\n", r.Discipline, r.Kind, r.Status)
		if r.Location != "" {
			fmt.Fprintf(&b, "- Location: %s\n", raceMarkdownCell(r.Location))
		}
		b.WriteString("\n")
	}
	if has("goals") && len(r.Goals) > 0 {
		b.WriteString("## Goals\n\n| Goal | Target | Achieved |\n| --- | --- | --- |\n")
		for _, g := range r.Goals {
			target, outcome := "", ""
			if g.TimeMS != nil {
				target = raceDuration(*g.TimeMS)
				if detail.Metrics.TimeMS != nil && r.Status == "finished" {
					if *detail.Metrics.TimeMS <= *g.TimeMS {
						outcome = "Yes"
					} else {
						outcome = "No"
					}
				}
			} else if g.Achieved != nil {
				if *g.Achieved {
					outcome = "Yes"
				} else {
					outcome = "No"
				}
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", raceMarkdownCell(g.Name), target, outcome)
		}
		b.WriteString("\n")
	}
	if has("results") {
		b.WriteString("## Results\n\n")
		for _, v := range []struct {
			label string
			ms    *int64
		}{{"Chip time", r.Result.ChipTimeMS}, {"Gun time", r.Result.GunTimeMS}, {"Manual time", r.Result.ManualTimeMS}} {
			if v.ms != nil {
				fmt.Fprintf(&b, "- %s: %s\n", v.label, raceDuration(*v.ms))
			}
		}
		if r.Result.OverallPlace != nil {
			fmt.Fprintf(&b, "- Overall place: %d\n", *r.Result.OverallPlace)
		}
		if r.Result.CategoryPlace != nil {
			fmt.Fprintf(&b, "- Category place (%s): %d\n", raceMarkdownCell(r.Result.Category), *r.Result.CategoryPlace)
		}
		if detail.Metrics.VDOT != nil {
			fmt.Fprintf(&b, "- VDOT: %.2f\n", *detail.Metrics.VDOT)
		}
		if detail.Metrics.AgeGrade != nil {
			fmt.Fprintf(&b, "- Age grade: %.2f%% (%s)\n", *detail.Metrics.AgeGrade, detail.Metrics.TableVersion)
		}
		b.WriteString("\n")
	}
	if has("checkpoints") && len(r.Result.Checkpoints) > 0 {
		fmt.Fprintf(&b, "## Official checkpoints (%s time)\n\n| Checkpoint | Distance | Cumulative time |\n| --- | --- | --- |\n", r.Result.SplitBasis)
		for _, p := range r.Result.Checkpoints {
			fmt.Fprintf(&b, "| %s | %g km | %s |\n", raceMarkdownCell(p.Name), p.DistanceM/1000, raceDuration(p.TimeMS))
		}
		b.WriteString("\n")
	}
	if has("laps") && detail.Activity != nil && len(detail.Activity.Laps) > 0 {
		b.WriteString("## Recorded activity laps (elapsed time)\n\n| Lap | Distance | Lap time |\n| --- | --- | --- |\n")
		for _, lap := range detail.Activity.Laps {
			fmt.Fprintf(&b, "| %d | %g km | %s |\n", lap.Index+1, lap.DistanceM/1000, raceDuration(int64(lap.ElapsedTimeS)*1000))
		}
		b.WriteString("\n")
	}
	h := detail.Halfway
	if has("halfway") && h.FirstHalfMS != nil {
		fmt.Fprintf(&b, "## Halfway analysis\n\n- Source: %s (%s time)\n- First half: %s\n", h.Source, h.Basis, raceDuration(*h.FirstHalfMS))
		if h.SecondHalfMS != nil {
			fmt.Fprintf(&b, "- Second half: %s\n- Second minus first: %s\n", raceDuration(*h.SecondHalfMS), raceDuration(*h.DifferenceMS))
		}
		if h.Scaled {
			b.WriteString("- Estimated recorded-distance midpoint; elapsed times scaled to the confirmed finish time.\n")
		}
		b.WriteString("\n")
	}
	if has("buildup") && len(detail.BuildUp) > 0 {
		b.WriteString("## Training build-up\n\n| Week starting | Running | Duration | Elevation | Runs | Longest run |\n| --- | --- | --- | --- | --- | --- |\n")
		for _, w := range detail.BuildUp {
			fmt.Fprintf(&b, "| %s | %.1f km | %s | %.0f m | %d | %.1f km |\n", w.From, w.DistanceM/1000, raceDuration(int64(w.DurationS)*1000), w.ElevationM, w.Count, w.LongestM/1000)
		}
		b.WriteString("\n")
	}
	if has("weather") && detail.Forecast.FetchedAt != nil {
		fmt.Fprintf(&b, "## Forecast\n\nForecast fetched %s. Forecasts are distinct from observed activity weather. Weather data: [Open-Meteo](https://open-meteo.com/), CC BY 4.0.\n\n", detail.Forecast.FetchedAt.Format("2006-01-02 15:04 MST"))
		b.WriteString("| Time (UTC) | Temperature | Feels like | Rain probability | Wind |\n| --- | --- | --- | --- | --- |\n")
		number := func(v *float64, suffix string) string {
			if v == nil {
				return ""
			}
			return fmt.Sprintf("%.1f%s", *v, suffix)
		}
		for _, hour := range detail.Forecast.Hours {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", hour.Time.UTC().Format("15:04"), number(hour.TemperatureC, " °C"), number(hour.ApparentTemperatureC, " °C"), number(hour.RainProbability, "%"), number(hour.WindKPH, " km/h"))
		}
		b.WriteString("\n")
	}
	if has("narrative") {
		for _, section := range [][2]string{{"Training", draft.Training}, {"Preparation", draft.Preparation}, {"Race experience", draft.Experience}, {"Reflections", draft.Reflections}} {
			if strings.TrimSpace(section[1]) != "" {
				fmt.Fprintf(&b, "## %s\n\n%s\n\n", section[0], section[1])
			}
		}
	}
	return b.String()
}
func (s *Server) handleRaceReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := s.store.RaceDetail(r.Context(), id)
	if s.writeRaceError(w, err) {
		return
	}
	draft, err := s.store.RaceReport(r.Context(), id)
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"draft": draft, "markdown": raceReportMarkdown(detail, draft)})
}
func (s *Server) handleSaveRaceReport(w http.ResponseWriter, r *http.Request) {
	var draft RaceReport
	if err := decodeJSONBody(r, &draft); err != nil {
		writeError(w, 400, "invalid report")
		return
	}
	saved, err := s.store.SaveRaceReport(r.Context(), chi.URLParam(r, "id"), draft)
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, saved)
}
func (s *Server) handlePreviewRaceReport(w http.ResponseWriter, r *http.Request) {
	var draft RaceReport
	if err := decodeJSONBody(r, &draft); err != nil {
		writeError(w, 400, "invalid report")
		return
	}
	if s.writeRaceError(w, validateRaceReport(draft)) {
		return
	}
	detail, err := s.store.RaceDetail(r.Context(), chi.URLParam(r, "id"))
	if s.writeRaceError(w, err) {
		return
	}
	writeJSON(w, 200, map[string]string{"markdown": raceReportMarkdown(detail, draft)})
}
