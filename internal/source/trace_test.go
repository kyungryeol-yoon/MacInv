package source

import "testing"

func TestClassify(t *testing.T) {
	env := TraceEnv{GoBin: "/Users/cris/go/bin", Home: "/Users/cris"}
	cases := []struct {
		name        string
		resolved    string
		wantManager string
		wantPkg     string
	}{
		{"brew formula", "/opt/homebrew/Cellar/git/2.53.0_1/bin/git", MgrBrew, "git"},
		{"brew cask", "/opt/homebrew/Caskroom/iterm2/3.5.6/iTerm.app", MgrBrew, "iterm2"},
		{"npm scoped", "/Users/cris/.nvm/versions/node/v22.22.2/lib/node_modules/@google/gemini-cli/dist/index.js", MgrNpm, "@google/gemini-cli"},
		{"npm plain", "/usr/local/lib/node_modules/npm/bin/npm-cli.js", MgrNpm, "npm"},
		{"pipx venv", "/Users/cris/.local/pipx/venvs/black/bin/black", MgrPipx, "black"},
		{"cargo", "/Users/cris/.cargo/bin/rg", MgrCargo, ""},
		{"go bin", "/Users/cris/go/bin/gopls", MgrGo, ""},
		{"system", "/usr/bin/git", MgrSystem, ""},
		{"system sbin", "/sbin/ping", MgrSystem, ""},
		{"manual", "/Users/cris/bin/mytool", MgrManual, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mgr, pkg := Classify(c.resolved, env)
			if mgr != c.wantManager || pkg != c.wantPkg {
				t.Errorf("Classify(%q) = (%s, %q), want (%s, %q)", c.resolved, mgr, pkg, c.wantManager, c.wantPkg)
			}
		})
	}
}
