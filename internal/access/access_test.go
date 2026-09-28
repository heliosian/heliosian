package access

import "testing"

func TestTheHatGrantsActingAllowancesOnly(t *testing.T) {
	see, act := Standing("test.see"), Acting("test.act")
	held := []Allowance{see, act}
	off := Actor{Allowances: Grant(held, false)}
	if !off.May(see) || off.May(act) {
		t.Fatalf("hat off: see %v, act %v", off.May(see), off.May(act))
	}
	on := Actor{Allowances: Grant(held, true)}
	if !on.May(see) || !on.May(act) {
		t.Fatalf("hat on: see %v, act %v", on.May(see), on.May(act))
	}
	if (Actor{Allowances: Grant(nil, true)}).May(see) {
		t.Fatal("the hat granted an allowance nobody held")
	}
}
