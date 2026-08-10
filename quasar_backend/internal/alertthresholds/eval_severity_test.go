package alertthresholds

import "testing"

func TestEvalOne_GTETemperature(t *testing.T) {
	m := thresholdMetric{
		Operator: "gte", WarningMin: 60, CriticalMin: 75,
		HasWarning: true, HasCritical: true,
	}
	if evalOne(50, m) != "ok" {
		t.Fatal("50°C should be ok")
	}
	if evalOne(60, m) != "warning" {
		t.Fatal("60°C should be warning")
	}
	if evalOne(80, m) != "critical" {
		t.Fatal("80°C should be critical")
	}
}

func TestEvalOne_LTEOptical(t *testing.T) {
	m := thresholdMetric{
		Operator: "lte", WarningMin: -15, CriticalMin: -20,
		HasWarning: true, HasCritical: true,
	}
	if evalOne(-10, m) != "ok" {
		t.Fatal("-10 dBm should be ok")
	}
	if evalOne(-15, m) != "warning" {
		t.Fatal("-15 dBm should be warning")
	}
	if evalOne(-22, m) != "critical" {
		t.Fatal("-22 dBm should be critical")
	}
}

func TestSeverityGteMetric_Operators(t *testing.T) {
	gte := GteMetricThreshold{Operator: "gte", Warning: 60, Critical: 75, HasWarn: true, HasCrit: true}
	if severityGteMetric(70, gte) != "warning" {
		t.Fatal("gte warning")
	}
	lte := GteMetricThreshold{Operator: "lte", Warning: -15, Critical: -20, HasWarn: true, HasCrit: true}
	if severityGteMetric(-18, lte) != "warning" {
		t.Fatal("lte warning")
	}
}
