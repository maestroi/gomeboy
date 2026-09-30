package types

import "testing"

func TestCGBRevisionFamilies(t *testing.T) {
	tests := []struct {
		model Model
		name  string
		cgb   bool
		exact bool
	}{
		{CGB0, "CGB0", true, false},
		{CGBABC, "CGB", true, false},
		{CGBBC, "CGBBC", true, true},
		{CGBDE, "CGBDE", true, true},
		{DMGABC, "DMG", false, false},
		{AGB, "AGB", false, false},
	}
	for _, tc := range tests {
		if got := tc.model.String(); got != tc.name {
			t.Errorf("%v String() = %q, want %q", tc.model, got, tc.name)
		}
		if got := tc.model.IsCGB(); got != tc.cgb {
			t.Errorf("%s IsCGB() = %v, want %v", tc.name, got, tc.cgb)
		}
		if got := tc.model.IsExactCGBRevision(); got != tc.exact {
			t.Errorf("%s IsExactCGBRevision() = %v, want %v", tc.name, got, tc.exact)
		}
		if got := StringToModel(tc.name); got != tc.model {
			t.Errorf("StringToModel(%q) = %v, want %v", tc.name, got, tc.model)
		}
	}
}

func TestCGBRevisionBootStateMatchesGenericProfile(t *testing.T) {
	for _, m := range []Model{CGBBC, CGBDE} {
		if got, want := ModelIO[m], ModelIO[CGBABC]; len(got) != len(want) {
			t.Errorf("%s ModelIO entries = %d, want %d", m, len(got), len(want))
		}
		if len(ModelRegisters[m]) != len(ModelRegisters[CGBABC]) {
			t.Errorf("%s ModelRegisters missing generic CGB boot profile", m)
		}
		if len(ModelRegistersCGB[m]) != len(ModelRegistersCGB[CGBABC]) {
			t.Errorf("%s ModelRegistersCGB missing generic CGB boot profile", m)
		}
	}
}
