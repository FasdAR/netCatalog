# Cobra CLI Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans for inline execution, or superpowers:subagent-driven-development if the user selects delegation. Steps use checkbox syntax for tracking.

**Goal:** Перевести приложение на стандартные глобальные команды cobra-cli и полностью убрать internal/cli.

**Architecture:** Единственный main.go вызывает cmd.Execute(ctx). Глобальные команды регистрируются через init(), каждый уровень дерева находится в отдельном файле. Провайдеры и resolver сохраняются; тестовый помощник изолирует повторные выполнения глобального дерева.

**Tech Stack:** Go 1.22, Cobra v1.10.2, cobra-cli, Testify, httptest, golangci-lint v2.14.0.

**Spec:** docs/superpowers/specs/2026-10-09-cobra-cli-migration-design.md

## Global Constraints

- Полностью убрать internal/cli/ и cmd/netCatalog/main.go после переноса.
- internal/resolver/ и internal/providers/ не изменяются.
- Коды: 0 — успешный, частичный или пустой результат; 1 — ошибка выполнения или записи; 2 — неверный ввод, неизвестный или неоднозначный сервис.
- Данные выводятся в stdout, диагностика в stderr; предупреждения включаются только с --info.
- Сохраняются все существующие команды, значения флагов по умолчанию и ограничения.
- Completion остаётся отключённым. Шаблонный флаг toggle удаляется.
- Go 1.22 и зависимости приложения сохраняются; Viper не добавляется.
- Пользовательские изменения .gitignore, go.mod, go.sum и LICENSE не откатываются.
- Изменения не коммитятся автоматически без отдельного запроса пользователя.

## Review Focus

1. Значение --family и Changed не переходят в следующий тестовый вызов get domains.
2. Значение --site не заставляет следующий вызов обходить каталог.
3. Отменённый контекст одного тестового выполнения не отменяет следующее.
4. Сбой записи help не оставляет неисправный writer следующему вызову.
5. Одноимённые services list и sources list зарегистрированы у правильных родителей.

---

### Task 1: Перенос команд и контрактов выполнения

**Files:**
- Modify: cmd/root.go, main.go.
- Create: cmd/execute.go, cmd/catalogue.go, cmd/warnings.go, cmd/get.go,
  cmd/services.go, cmd/services_list.go, cmd/services_search.go,
  cmd/sources.go, cmd/sources_list.go.
- Move and adapt: internal/cli/cli_test.go → cmd/cli_test.go,
  internal/cli/regression_test.go → cmd/regression_test.go,
  internal/cli/warnings_test.go → cmd/warnings_test.go,
  internal/cli/rename_test.go → cmd/rename_test.go.
- Create: cmd/test_helpers_test.go.
- Delete after passing tests: internal/cli/*.go, cmd/netCatalog/main.go.

**Interfaces:**
- Consumes: existing resolver/providers APIs without modifications.
- Produces: Execute(ctx context.Context) int for main;
  run(ctx context.Context, args []string, stdout, stderr io.Writer) int internally;
  rootCmd, getCmd, servicesCmd, servicesListCmd, servicesSearchCmd,
  sourcesCmd, sourcesListCmd as *cobra.Command globals.
- Shared options type keeps url, v2flyURL, timeout, info, color;
  global rootOptions *options is bound to persistent flags.
- Preserve options.engine(includeV2fly bool), options.warnings(io.Writer),
  loadCatalogue(context.Context, resolver.Resolver, warningPrinter),
  listServices(*cobra.Command, *options, string) and runtimeError(error).
- Test-only runForTest(t *testing.T, ctx context.Context, args []string,
  stdout, stderr io.Writer, defaultV2flyURL string) int resets global state
  and calls run. Tests of the exported Execute use process streams only
  if needed; existing integration tests use runForTest.

- [ ] Run baseline `go test -race ./...`; record existing failures separately.
- [ ] Locate cobra-cli in PATH or `go env GOPATH`/bin. If missing, install
  `github.com/spf13/cobra-cli@v1.3.0` to an approved temporary GOBIN without
  changing application dependencies. Do not rerun init or overwrite LICENSE.
- [ ] Copy all four CLI test files to cmd using package cmd. Replace internal
  run/Run invocations with runForTest and keep assertions and local HTTP fixtures.
  Existing execute helper passes an empty defaultV2flyURL to avoid live HTTP.
- [ ] Run `go test ./cmd`; expect compilation failure because runForTest is absent.
- [ ] Generate missing command files with cobra-cli add get/services/sources,
  then servicesList -p servicesCmd, servicesSearch -p servicesCmd,
  sourcesList -p sourcesCmd. Use --license none for new scaffolds. Rename
  generated camelCase child files to snake_case paths listed above.
- [ ] Replace cmd/root.go scaffold with existing root metadata, Args, RunE,
  PersistentPreRunE and flags bound to rootOptions. Keep SilenceErrors,
  SilenceUsage and disabled completion; remove toggle and placeholder copy.
- [ ] Transfer each existing command body to its global command, registering
  child commands in init(). Child Use strings remain list, search TERM, list;
  get uses get SERVICE. Retain existing validation and output behavior.
- [ ] Extract provider assembly and catalogue loading into cmd/catalogue.go;
  move warning output unchanged into cmd/warnings.go.
- [ ] Transfer checkedWriter, commandError and run behavior to cmd/execute.go.
  Execute uses os.Args[1:], os.Stdout and os.Stderr; run configures rootCmd
  streams, arguments, color detection and context before ExecuteContext.
  Preserve write-error checking for help and single error reporting.
- [ ] Implement runForTest and recursive reset in cmd/test_helpers_test.go.
  Walk each command's own Flags and PersistentFlags; set values through
  flag.Value.Set(flag.DefValue), then clear Changed. Check reset errors.
  Reset execution arguments, streams and context before each call, override
  rootOptions.v2flyURL for fixtures, and register cleanup to reset state.
  Do not run tests using global commands in parallel. Adapt the existing
  repeated --info scenario to run through this helper.
- [ ] Run `go test -race ./cmd`; expect all transferred tests to pass.
- [ ] Update main.go to use signal.NotifyContext, call cmd.Execute(ctx), call
  stop(), then os.Exit(code); remove generated author placeholders without
  changing LICENSE.
- [ ] Delete only the migrated internal/cli files and old cmd/netCatalog/main.go.
- [ ] Run `go test -race ./...`; expect all packages to pass.

### Task 2: Проверка изоляции и регистрации глобального дерева

**Files:** cmd/regression_test.go, cmd/test_helpers_test.go.

**Interfaces:** consumes Task 1 runForTest and the migrated HTTP fixtures;
produces additional regression coverage, no new production API.

- [ ] Add TestRunDoesNotRetainFamilyFlag: first get youtube --site --type ips
  --family ipv6, then get youtube --site --type domains on youtubeServer;
  assert second code 0 and output "youtube.com\n". Both calls use runForTest.
- [ ] Add TestRunDoesNotRetainSiteFlag: use a server counting catalogue requests;
  first get youtube.com --site --type domains, then get youtube --type domains;
  assert second call loads the catalogue and succeeds.
- [ ] Add TestRunDoesNotRetainCancelledContext: first call services list with
  a cancelled context and local source, assert code 1; second call with
  context.Background on a working fixture, assert code 0 and nonempty output.
- [ ] Add TestRunDoesNotRetainFailedOutput: first --help with failingWriter,
  assert code 1; second --help with buffers, assert code 0, nonempty output
  and empty stderr.
- [ ] Add TestRootRegistersNestedListCommands: use rootCmd.Find for
  [services,list] and [sources,list], assert no errors, exact command pointers
  servicesListCmd/sourcesListCmd and matching Parent pointers.
- [ ] Run `go test -race ./cmd -run 'TestRunDoesNotRetain|TestRootRegisters' -count=5`.
  Expect pass. If a new regression fails, correct only the test isolation or
  registration responsible, then rerun; do not add runtime reset machinery.
- [ ] Run `go test -race ./... -count=2`; expect pass without external HTTP.

### Task 3: Сборка, документация и итоговая проверка

**Files:** Makefile, README.md.

**Interfaces:** root package is the sole build target; fmt targets main.go,
cmd and internal. No additional exported Go interfaces.

- [ ] Update Makefile fmt and fmt-check to include main.go alongside cmd/internal.
- [ ] Update README build/run instructions to `go build -o bin/netCatalog .`
  and `go run .`; replace old structure descriptions and document generation
  commands, unique Go names for nested list commands, and init registration.
- [ ] Format changed Go files and run `make fmt-check`; expect no differences.
- [ ] Run `go test -race ./...` and `go vet ./...`; expect successful exits.
- [ ] Run `go build -o bin/netCatalog .`; expect successful exit.
- [ ] Run `bin/netCatalog --help`, `bin/netCatalog get --help`,
  `bin/netCatalog services search --help`, `bin/netCatalog sources list`;
  expect code 0, correct command names/flags and two supported sources.
- [ ] Run `make lint` using v2.14.0 if available. Report any unavailable
  tool explicitly rather than claiming its checks passed.
- [ ] Inspect git diff and search active source/docs for internal/cli and
  cmd/netCatalog; historical docs may keep old paths, active README may not.
  Verify user changes remain intact and no generated toggle/placeholder
  descriptions or extra dependencies were introduced.

## Handoff

Specification is approved. Implementation starts after the user reviews this
plan and chooses inline execution or subagent-driven execution. No automatic
commits or unrelated cleanup.
