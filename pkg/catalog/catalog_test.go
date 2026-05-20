package catalog

import "testing"

func TestLoadDefault(t *testing.T) {
	cat, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error: %v", err)
	}
	if cat.Metadata.Name != "docsclaw-default" {
		t.Errorf("name = %q, want docsclaw-default", cat.Metadata.Name)
	}
	if _, ok := cat.Tools["curl"]; !ok {
		t.Error("curl not in default catalog")
	}
	if _, ok := cat.Tools["jq"]; !ok {
		t.Error("jq not in default catalog")
	}
}

func TestLoadFromFile(t *testing.T) {
	cat, err := LoadFromFile("testdata/custom-catalog.yaml")
	if err != nil {
		t.Fatalf("LoadFromFile() error: %v", err)
	}
	if cat.Metadata.Name != "test-catalog" {
		t.Errorf("name = %q, want test-catalog", cat.Metadata.Name)
	}
}

func TestLookupTool(t *testing.T) {
	cat, _ := LoadDefault()

	entry, ok := cat.Lookup("curl")
	if !ok {
		t.Fatal("curl not found")
	}
	if entry.Tier != "core" {
		t.Errorf("curl tier = %q, want core", entry.Tier)
	}

	_, ok = cat.Lookup("nonexistent")
	if ok {
		t.Error("nonexistent tool should not be found")
	}
}

func TestCoreTierTools(t *testing.T) {
	cat, _ := LoadDefault()
	core := cat.CoreTools()
	if len(core) == 0 {
		t.Fatal("no core tools found")
	}
	for _, name := range core {
		entry, _ := cat.Lookup(name)
		if entry.Tier != "core" {
			t.Errorf("%s tier = %q, want core", name, entry.Tier)
		}
	}
}

func TestHighestTier(t *testing.T) {
	cat, _ := LoadDefault()

	tests := []struct {
		tools []string
		want  string
	}{
		{[]string{"curl", "jq"}, "core"},
		{[]string{"curl", "git"}, "standard"},
		{[]string{"curl", "pandoc"}, "extended"},
		{[]string{"curl", "python3"}, "runtime"},
	}
	for _, tt := range tests {
		got := cat.HighestTier(tt.tools)
		if got != tt.want {
			t.Errorf("HighestTier(%v) = %q, want %q", tt.tools, got, tt.want)
		}
	}
}

func TestMaxRiskScore(t *testing.T) {
	cat, _ := LoadDefault()
	score := cat.MaxRiskScore([]string{"curl", "jq"})
	if score < 1 || score > 3 {
		t.Errorf("core-only risk score = %d, expected 1-3", score)
	}

	score = cat.MaxRiskScore([]string{"curl", "python3"})
	if score < 7 {
		t.Errorf("python3 risk score = %d, expected >= 7", score)
	}
}

func TestPackageName(t *testing.T) {
	cat, _ := LoadDefault()
	entry, _ := cat.Lookup("openssh-client")
	if entry.Package["dnf"] != "openssh-clients" {
		t.Errorf("openssh-client dnf package = %q, want openssh-clients", entry.Package["dnf"])
	}
}

func TestPackageNames(t *testing.T) {
	cat, _ := LoadDefault()

	dnfPkgs := cat.PackageNames([]string{"curl", "jq", "openssh-client"}, "dnf")
	if len(dnfPkgs) != 3 {
		t.Errorf("dnf packages = %d, want 3", len(dnfPkgs))
	}

	apkPkgs := cat.PackageNames([]string{"curl", "jq"}, "apk")
	if len(apkPkgs) != 2 {
		t.Errorf("apk packages = %d, want 2", len(apkPkgs))
	}

	// Unknown tool should be skipped
	pkgs := cat.PackageNames([]string{"curl", "unknown"}, "dnf")
	if len(pkgs) != 1 {
		t.Errorf("packages with unknown = %d, want 1", len(pkgs))
	}
}

func TestValidate(t *testing.T) {
	cat, _ := LoadDefault()

	// Valid tools
	err := cat.Validate([]string{"curl", "jq", "git"})
	if err != nil {
		t.Errorf("Validate() with valid tools: %v", err)
	}

	// Invalid tool
	err = cat.Validate([]string{"curl", "unknown"})
	if err == nil {
		t.Error("Validate() with unknown tool should error")
	}
}
