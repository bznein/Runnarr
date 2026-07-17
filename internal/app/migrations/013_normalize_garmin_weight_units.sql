with raw_weights as (
	select
		provider,
		metric_date,
		nullif(coalesce(
			raw #>> '{bodyComposition,totalAverage,weight}',
			raw #>> '{bodyComposition,dateWeightList,0,weight}',
			raw #>> '{statsAndBody,totalAverage,weight}',
			raw #>> '{statsAndBody,dateWeightList,0,weight}'
		), '')::double precision as raw_weight
	from daily_health_metrics
	where provider = 'garmin'
)
update daily_health_metrics as metrics
set weight_kg = case
	when raw_weights.raw_weight > 1000 then raw_weights.raw_weight / 1000
	else raw_weights.raw_weight
end
from raw_weights
where metrics.provider = raw_weights.provider
	and metrics.metric_date = raw_weights.metric_date
	and raw_weights.raw_weight is not null
	and (metrics.weight_kg is null or metrics.weight_kg > 1000);

update daily_health_metrics
set weight_kg = weight_kg / 1000
where provider = 'garmin'
	and weight_kg > 1000;
