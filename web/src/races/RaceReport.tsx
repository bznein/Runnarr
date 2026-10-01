import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { raceApi } from "./api";
import {
  RaceError,
  RaceField,
  RaceLoading,
  useInvalidateRaces,
} from "./components";
import type { RaceReport } from "./types";

const sections = {
  facts: "Race information",
  goals: "Goals",
  results: "Results",
  checkpoints: "Official checkpoints",
  laps: "Watch laps",
  halfway: "Halfway analysis",
  buildup: "Training build-up",
  weather: "Forecast",
  narrative: "Written report",
};
export function RaceReportPage({ canWrite }: { canWrite: boolean }) {
  const { id } = useParams();
  const query = useQuery({
    queryKey: ["races", "report", id],
    queryFn: () => raceApi.report(id!),
  });
  if (query.isLoading) return <RaceLoading />;
  if (!query.data) return <RaceError error={query.error} />;
  return (
    <ReportEditor
      key={id}
      id={id!}
      initial={query.data.draft}
      initialMarkdown={query.data.markdown}
      canWrite={canWrite}
    />
  );
}
function ReportEditor({
  id,
  initial,
  initialMarkdown,
  canWrite,
}: {
  id: string;
  initial: RaceReport;
  initialMarkdown: string;
  canWrite: boolean;
}) {
  const [draft, setDraft] = useState(initial);
  const [markdown, setMarkdown] = useState(initialMarkdown);
  const [changed, setChanged] = useState(false);
  const [message, setMessage] = useState("");
  const invalidate = useInvalidateRaces();
  const preview = useMutation({
    mutationFn: () => raceApi.previewReport(id, draft),
    onSuccess: (data) => {
      setMarkdown(data.markdown);
      setChanged(false);
      setMessage("Preview updated");
    },
  });
  const save = useMutation({
    mutationFn: async () => {
      const saved = await raceApi.saveReport(id, draft);
      setDraft(saved);
      return raceApi.previewReport(id, saved);
    },
    onSuccess: async (data) => {
      setMarkdown(data.markdown);
      setChanged(false);
      setMessage("Report saved");
      await invalidate();
    },
  });
  const update = (patch: Partial<RaceReport>) => {
    setDraft((old) => ({ ...old, ...patch }));
    setChanged(true);
    setMessage("");
  };
  async function copy() {
    try {
      await navigator.clipboard.writeText(markdown);
      setMessage("Markdown copied");
    } catch {
      setMessage("Clipboard unavailable. Download the Markdown instead.");
    }
  }
  function download() {
    const url = URL.createObjectURL(
      new Blob([markdown], { type: "text/markdown;charset=utf-8" }),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = `race-report-${id}.md`;
    a.click();
    URL.revokeObjectURL(url);
  }
  function print() {
    document.body.classList.add("race-printing");
    window.addEventListener(
      "afterprint",
      () => document.body.classList.remove("race-printing"),
      { once: true },
    );
    window.print();
  }
  return (
    <>
      <section className="panel race-section race-report-controls">
        <div className="race-heading">
          <h2>Race report</h2>
          <Link to={`/races/${id}`} className="secondary-button">
            Back to race
          </Link>
        </div>
        <p className="muted">
          Choose sections and write your own account. Event logistics and birth
          date are excluded. Review the preview before sharing.
        </p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <fieldset disabled={!canWrite || save.isPending || preview.isPending}>
            <div className="race-report-options">
              {Object.entries(sections).map(([key, label]) => (
                <label className="race-check" key={key}>
                  <input
                    type="checkbox"
                    checked={draft.sections.includes(key)}
                    onChange={(e) =>
                      update({
                        sections: e.target.checked
                          ? [...draft.sections, key]
                          : draft.sections.filter((s) => s !== key),
                      })
                    }
                  />
                  {label}
                </label>
              ))}
            </div>
            {(
              [
                ["training", "Training"],
                ["preparation", "Preparation"],
                ["experience", "Race experience"],
                ["reflections", "Reflections"],
              ] as const
            ).map(([key, label]) => (
              <RaceField key={key} label={label}>
                <textarea
                  rows={5}
                  maxLength={20000}
                  value={draft[key]}
                  onChange={(e) => update({ [key]: e.target.value })}
                />
              </RaceField>
            ))}
            <div className="race-actions">
              <button className="primary-button">Save report</button>
              <button
                type="button"
                className="secondary-button"
                onClick={() => preview.mutate()}
              >
                Update preview
              </button>
            </div>
          </fieldset>
        </form>
        <RaceError error={save.error ?? preview.error} />
        <p role="status" className="muted">
          {message ||
            (changed ? "Preview needs updating to include your edits." : "")}
        </p>
        <div className="race-actions">
          <button
            className="secondary-button"
            disabled={changed || save.isPending || preview.isPending}
            onClick={() => void copy()}
          >
            Copy Markdown
          </button>
          <button
            className="secondary-button"
            disabled={changed || save.isPending || preview.isPending}
            onClick={download}
          >
            Download Markdown
          </button>
          <button
            className="secondary-button"
            disabled={changed || save.isPending || preview.isPending}
            onClick={print}
          >
            Print / save PDF
          </button>
        </div>
      </section>
      <article
        className="panel race-report-preview"
        aria-label="Report preview"
      >
        <MarkdownPreview markdown={markdown} />
      </article>
    </>
  );
}
// A deliberately small safe renderer: authored text never becomes HTML or executable links.
export function MarkdownPreview({ markdown }: { markdown: string }) {
  const blocks = markdown.split(/\n\s*\n/);
  const plain = (text: string) =>
    text
      .replace(/\\([\\|\[\]*_`])/g, "$1")
      .replace(/&lt;/g, "<")
      .replace(/&gt;/g, ">");
  return (
    <>
      {blocks.map((block, i) => {
        if (block.startsWith("# "))
          return <h1 key={i}>{plain(block.slice(2))}</h1>;
        if (block.startsWith("## "))
          return <h2 key={i}>{plain(block.slice(3))}</h2>;
        const lines = block.split("\n");
        if (lines.length > 1 && /^\|[\s|:-]+\|$/.test(lines[1])) {
          const cells = (line: string) =>
            line
              .replace(/^\||\|$/g, "")
              .split(/(?<!\\)\|/)
              .map((c) => plain(c.trim()));
          return (
            <div key={i} className="race-table-wrap">
              <table className="race-table">
                <thead>
                  <tr>
                    {cells(lines[0]).map((c, j) => (
                      <th key={j}>{c}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {lines.slice(2).map((line, j) => (
                    <tr key={j}>
                      {cells(line).map((c, k) => (
                        <td key={k}>{c}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          );
        }
        if (lines.every((line) => line.startsWith("- ")))
          return (
            <ul key={i}>
              {lines.map((line, j) => (
                <li key={j}>{plain(line.slice(2))}</li>
              ))}
            </ul>
          );
        return (
          <p key={i} style={{ whiteSpace: "pre-wrap" }}>
            {plain(block)}
          </p>
        );
      })}
    </>
  );
}
