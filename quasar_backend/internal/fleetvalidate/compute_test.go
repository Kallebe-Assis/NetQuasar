package fleetvalidate

import (
	"testing"
	"time"
)

func TestComputeBasic(t *testing.T) {
	prev, curr := 1000.0, 1520.0
	c, err := Compute(Input{Liters: 52, PricePerLiter: 6.10, OdometerPrev: &prev, OdometerCurr: &curr})
	if err != nil {
		t.Fatal(err)
	}
	if c.TotalAmount != 317.2 {
		t.Fatalf("total=%v", c.TotalAmount)
	}
	if c.KmDriven == nil || *c.KmDriven != 520 {
		t.Fatalf("km=%v", c.KmDriven)
	}
	if c.KmPerLiter == nil || *c.KmPerLiter < 9.9 || *c.KmPerLiter > 10.1 {
		t.Fatalf("kpl=%v", c.KmPerLiter)
	}
}

func TestComputeRejectOdometer(t *testing.T) {
	prev, curr := 1000.0, 900.0
	_, err := Compute(Input{Liters: 10, PricePerLiter: 5, OdometerPrev: &prev, OdometerCurr: &curr})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEvaluateAlertsTank(t *testing.T) {
	cap := 60.0
	alerts := EvaluateAlerts(VehicleCtx{TankCapacityLiters: &cap}, Input{Liters: 78, PricePerLiter: 6, FuelingAt: time.Now()}, Computed{}, Settings{})
	if len(alerts) != 1 || alerts[0].AlertType != "tank_overflow" {
		t.Fatalf("%+v", alerts)
	}
}
