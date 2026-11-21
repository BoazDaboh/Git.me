package discovery

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChecksUseDeclaredScriptsAndActualStack(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "web/package.json", `{"packageManager":"pnpm@10","dependencies":{"next":"16"},"scripts":{"lint":"echo NEVER_EXPORT_SCRIPT_BODY","typecheck":"tsc --noEmit","test":"echo \"Error: no test specified\" && exit 1","build":"next build"}}`)
	fixture(t, root, "node/package.json", `{"scripts":{"test:unit":"run-unit-tests","start":"node app.js"}}`)
	fixture(t, root, "php/composer.json", `{"scripts":{"test":["phpunit"],"lint":"phpcs","build":false}}`)
	fixture(t, root, "php/public/index.php", "<?php")
	fixture(t, root, "laravel/composer.json", `{"require":{"laravel/framework":"12"}}`)
	fixture(t, root, "laravel/artisan", "<?php")
	fixture(t, root, "plain/index.php", "<?php")
	fixture(t, root, "api/go.mod", "module fixture/api\ngo 1.27.4\n")
	result, err := Scan(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	byFolder := map[string][]Check{}
	for _, service := range result.Services {
		byFolder[service.RelativeFolder] = service.Checks
	}
	want := map[string][]string{"web": {"pnpm.cmd run lint", "pnpm.cmd run typecheck", "pnpm.cmd run build"}, "node": {"npm.cmd run test:unit"}, "php": {"php -l public/index.php", "composer validate", "composer run-script lint", "composer run-script test"}, "laravel": {"composer validate", "php artisan test"}, "plain": {"php -l index.php"}, "api": {"go test ./...", "go vet ./...", "go build ./..."}}
	for folder, commands := range want {
		checks := byFolder[folder]
		if len(checks) != len(commands) {
			t.Fatal(folder, checks)
		}
		for i, command := range commands {
			if checks[i].Command != command {
				t.Fatal(folder, checks)
			}
		}
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "NEVER_EXPORT_SCRIPT_BODY") {
		t.Fatal("script contents exposed")
	}
}
