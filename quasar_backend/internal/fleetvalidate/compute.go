package fleetvalidate

import (
	"fmt"
	"math"
	"time"
)

type Settings struct {
	ConsumptionTolerancePct float64
	PriceTolerancePct       float64
	MinMinutesBetween       int
}

type VehicleCtx struct {
	TankCapacityLiters  *float64
	ExpectedKmPerLiter  *float64
	MinKmPerLiter       *float64
	MaxKmPerLiter       *float64
	OdometerCurrent     float64
}

type Input struct {
	Liters         float64
	PricePerLiter  float64
	OdometerPrev   *float64
	OdometerCurr   *float64
	HourmeterPrev  *float64
	HourmeterCurr  *float64
	LastFuelingAt  *time.Time
	FuelingAt      time.Time
	AvgPrice       *float64 // média posto/combustível (opcional)
}

type Computed struct {
	TotalAmount     float64
	KmDriven        *float64
	HoursWorked     *float64
	KmPerLiter      *float64
	CostPerKm       *float64
	LitersPer100Km  *float64
}

type AlertDraft struct {
	Severity  string
	AlertType string
	Title     string
	Message   string
}

func Compute(in Input) (Computed, error) {
	out := Computed{TotalAmount: round2(in.Liters * in.PricePerLiter)}
	if in.OdometerCurr != nil && in.OdometerPrev != nil {
		if *in.OdometerCurr < *in.OdometerPrev {
			return out, fmt.Errorf("hodômetro atual não pode ser menor que o anterior")
		}
		km := *in.OdometerCurr - *in.OdometerPrev
		out.KmDriven = &km
		if km > 0 && in.Liters > 0 {
			kpl := km / in.Liters
			out.KmPerLiter = &kpl
			cpk := out.TotalAmount / km
			out.CostPerKm = &cpk
			l100 := (in.Liters / km) * 100
			out.LitersPer100Km = &l100
		}
	}
	if in.HourmeterCurr != nil && in.HourmeterPrev != nil {
		if *in.HourmeterCurr < *in.HourmeterPrev {
			return out, fmt.Errorf("horímetro actual não pode ser menor que o anterior")
		}
		h := *in.HourmeterCurr - *in.HourmeterPrev
		out.HoursWorked = &h
	}
	return out, nil
}

func EvaluateAlerts(veh VehicleCtx, in Input, comp Computed, settings Settings) []AlertDraft {
	var alerts []AlertDraft
	if veh.TankCapacityLiters != nil && in.Liters > *veh.TankCapacityLiters {
		alerts = append(alerts, AlertDraft{
			Severity:  "critical",
			AlertType: "tank_overflow",
			Title:     "Abastecimento acima da capacidade do tanque",
			Message:   fmt.Sprintf("%.2f L abastecidos; capacidade do tanque: %.2f L.", in.Liters, *veh.TankCapacityLiters),
		})
	}
	if in.LastFuelingAt != nil && settings.MinMinutesBetween > 0 {
		diff := in.FuelingAt.Sub(*in.LastFuelingAt)
		if diff >= 0 && diff < time.Duration(settings.MinMinutesBetween)*time.Minute {
			alerts = append(alerts, AlertDraft{
				Severity:  "attention",
				AlertType: "fuelings_too_close",
				Title:     "Abastecimentos muito próximos",
				Message:   fmt.Sprintf("Novo abastecimento %.0f minutos após o anterior (mínimo configurado: %d min).", diff.Minutes(), settings.MinMinutesBetween),
			})
		}
	}
	if comp.KmPerLiter != nil {
		kpl := *comp.KmPerLiter
		minOK := veh.MinKmPerLiter
		maxOK := veh.MaxKmPerLiter
		if minOK == nil && veh.ExpectedKmPerLiter != nil && settings.ConsumptionTolerancePct > 0 {
			tol := settings.ConsumptionTolerancePct / 100
			v := *veh.ExpectedKmPerLiter * (1 - tol)
			minOK = &v
		}
		if maxOK == nil && veh.ExpectedKmPerLiter != nil && settings.ConsumptionTolerancePct > 0 {
			tol := settings.ConsumptionTolerancePct / 100
			v := *veh.ExpectedKmPerLiter * (1 + tol)
			maxOK = &v
		}
		if minOK != nil && kpl < *minOK {
			alerts = append(alerts, AlertDraft{
				Severity:  "critical",
				AlertType: "abnormal_consumption",
				Title:     "Consumo anormal (baixo KM/L)",
				Message:   fmt.Sprintf("Consumo actual: %.2f KM/L; mínimo aceitável: %.2f KM/L.", kpl, *minOK),
			})
		}
		if maxOK != nil && kpl > *maxOK {
			alerts = append(alerts, AlertDraft{
				Severity:  "attention",
				AlertType: "abnormal_consumption_high",
				Title:     "Consumo acima do máximo esperado",
				Message:   fmt.Sprintf("Consumo actual: %.2f KM/L; máximo esperado: %.2f KM/L.", kpl, *maxOK),
			})
		}
	}
	if in.AvgPrice != nil && *in.AvgPrice > 0 && settings.PriceTolerancePct > 0 {
		limit := *in.AvgPrice * (1 + settings.PriceTolerancePct/100)
		if in.PricePerLiter > limit {
			pct := ((in.PricePerLiter / *in.AvgPrice) - 1) * 100
			alerts = append(alerts, AlertDraft{
				Severity:  "attention",
				AlertType: "price_outlier",
				Title:     "Preço acima da média",
				Message:   fmt.Sprintf("Preço R$ %.3f/L está %.0f%% acima da média R$ %.3f/L.", in.PricePerLiter, pct, *in.AvgPrice),
			})
		}
	}
	return alerts
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
