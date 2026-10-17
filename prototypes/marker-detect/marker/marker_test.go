package marker

import "testing"

func TestIDMapping(t *testing.T) {
	cases := []struct{ id, team, player int }{
		{0, 0, 0}, {11, 0, 11}, {12, 1, 0}, {17, 1, 5}, {71, 5, 11},
	}
	for _, c := range cases {
		if !IsValid(c.id) {
			t.Errorf("IsValid(%d) = false", c.id)
		}
		if Team(c.id) != c.team || Player(c.id) != c.player {
			t.Errorf("id %d: team %d player %d, want %d %d", c.id, Team(c.id), Player(c.id), c.team, c.player)
		}
		if Team(c.id)*PlayersPerTeam+Player(c.id) != c.id {
			t.Errorf("id %d does not round-trip", c.id)
		}
	}
}

func TestValidRange(t *testing.T) {
	if Count != 72 {
		t.Fatalf("Count = %d, want 72", Count)
	}
	for _, id := range []int{-1, 72, 99, 100} {
		if IsValid(id) {
			t.Errorf("IsValid(%d) = true", id)
		}
	}
}

func TestTargetID(t *testing.T) {
	if got := TargetID(17); got != "17" {
		t.Errorf("TargetID(17) = %q", got)
	}
}
