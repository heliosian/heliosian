package access

import "testing"

func TestGrantGivesWhatIsHeldAndNothingElse(t *testing.T) {
	see, act := Named("test.see"), Named("test.act")
	a := Actor{Allowances: Grant([]Allowance{see})}
	if !a.May(see) || a.May(act) {
		t.Fatalf("see %v, act %v", a.May(see), a.May(act))
	}
}
