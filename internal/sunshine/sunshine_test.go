package sunshine

import "testing"

func TestChooseTakesThePairingFromTheMachineThatAsked(t *testing.T) {
	waiting := []pending{{ID: "a", Address: "100.64.0.2"}, {ID: "b", Address: "::ffff:100.64.0.3"}}
	if p, err := choose(waiting, "100.64.0.3"); err != nil || p.ID != "b" {
		t.Errorf("by address: %+v %v", p, err)
	}
	if _, err := choose(waiting, "100.64.0.9"); err == nil {
		t.Error("two waiting and none from the asker must not guess")
	}
	if p, err := choose(waiting[:1], ""); err != nil || p.ID != "a" {
		t.Errorf("the only one waiting: %+v %v", p, err)
	}
	if _, err := choose(nil, "100.64.0.2"); err != ErrNoPairing {
		t.Errorf("none waiting: %v", err)
	}
}
