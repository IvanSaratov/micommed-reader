package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	urfave "github.com/urfave/cli/v3"
)

type usageError struct{ message string }

func (e usageError) Error() string           { return e.message }
func usage(format string, args ...any) error { return usageError{fmt.Sprintf(format, args...)} }

const helpTemplate = `{{.FullName}} — {{.Usage}}
Вызов: {{.FullName}} {{.ArgsUsage}}
{{if .VisibleCommands}}
Команды:
{{range .VisibleCommands}}  {{.Name}}  {{.Usage}}
{{end}}{{end}}
Параметры:
{{range .VisibleFlags}}{{if eq (index .Names 0) "help"}}  -h, --help  Показать справку
{{else}}  {{.}}
{{end}}{{end}}`

func command(name, description, args string, action urfave.ActionFunc) *urfave.Command {
	return &urfave.Command{Name: name, Usage: description, ArgsUsage: args, Action: action,
		HideHelpCommand: true, CustomRootCommandHelpTemplate: helpTemplate, CustomHelpTemplate: helpTemplate,
		OnUsageError: func(_ context.Context, c *urfave.Command, err error, _ bool) error {
			if f, ok := strings.CutPrefix(err.Error(), "flag provided but not defined: "); ok {
				return usage("неизвестный параметр --%s", strings.TrimLeft(f, "-"))
			}
			if f, ok := strings.CutPrefix(err.Error(), "flag needs an argument: "); ok {
				return usage("укажите значение для --%s", strings.TrimLeft(f, "-"))
			}
			return usage("ошибка в параметрах команды %s. Используйте --help", c.FullName())
		}}
}

func group(ctx context.Context, c *urfave.Command) error {
	if c.NArg() != 0 {
		return usage("неизвестная команда %q. Используйте %s --help", c.Args().First(), c.FullName())
	}
	if c == c.Root() {
		return urfave.ShowRootCommandHelp(c)
	}
	return urfave.ShowCommandHelp(ctx, c.Root(), c.Name)
}

// Run разбирает полный argv, включая имя программы. Только main завершает процесс.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root := command("healdata", "Чтение дневных записей HEALDATA", "КОМАНДА [ПАРАМЕТРЫ]", group)
	root.Writer, root.ErrWriter = stdout, stderr
	root.ExitErrHandler = func(context.Context, *urfave.Command, error) {}
	export := command("export", "Экспорт измерений и сводки", "ФОРМАТ [ПАРАМЕТРЫ] ПУТЬ", group)
	export.Commands = []*urfave.Command{leaf("json", "Один документ JSON"), leaf("csv", "Набор таблиц CSV в новой папке")}
	root.Commands = []*urfave.Command{leaf("inspect", "Проверить файлы и показать их состав"), leaf("summary", "Показать дневные показатели и сводку периода"), export}
	if err := root.Run(ctx, args); err != nil {
		if errors.Is(err, errPartial) {
			fmt.Fprintln(stderr, err)
			return 3
		}
		fmt.Fprintln(stderr, "Ошибка:", err)
		var bad usageError
		if errors.As(err, &bad) {
			return 2
		}
		return 1
	}
	return 0
}

func leaf(name, description string) *urfave.Command {
	cmd := command(name, description, "[ПАРАМЕТРЫ] ПУТЬ", execute)
	cmd.Flags = []urfave.Flag{&urfave.StringFlag{Name: "date", Usage: "Отчётная дата переименованного файла (YYYY-MM-DD)"}}
	if name != "inspect" {
		cmd.Flags = append(cmd.Flags, &urfave.StringFlag{Name: "from", Usage: "Первый отчётный день включительно"}, &urfave.StringFlag{Name: "to", Usage: "Последний отчётный день включительно"})
	}
	if name == "csv" || name == "json" {
		cmd.Flags = append(cmd.Flags, &urfave.StringFlag{Name: "out", Aliases: []string{"o"}, Usage: "Путь для нового результата"})
	}
	return cmd
}

var errPartial = errors.New("часть записей повреждена или исключена. Причины указаны в отчёте")

func execute(ctx context.Context, c *urfave.Command) error {
	name := c.Name
	if c.NArg() != 1 {
		return usage("укажите один путь к дневному файлу, папке или ZIP")
	}
	for _, flag := range []string{"date", "from", "to"} {
		if c.IsSet(flag) {
			if _, err := parseDate(c.String(flag)); err != nil {
				return usage("некорректная дата --%s %q, ожидается YYYY-MM-DD", flag, c.String(flag))
			}
		}
	}
	from, to := c.String("from"), c.String("to")
	if from != "" && to != "" && from > to {
		return usage("дата --from позже --to")
	}
	out := c.String("out")
	if out == "" && (name == "csv" || c.IsSet("out")) {
		return usage("укажите назначение через --out или -o")
	}
	input, err := readInput(ctx, c.Args().First(), c.String("date"))
	if err != nil {
		return err
	}
	report, err := makeReport(input, from, to)
	if err != nil {
		return err
	}

	stdout := c.Root().Writer
	switch name {
	case "inspect", "summary":
		err = report.writeText(stdout, name)
	case "json":
		if out == "" {
			err = report.writeJSON(stdout)
		} else {
			err = publishJSON(out, report.writeJSON)
		}
	case "csv":
		err = report.writeCSV(out)
	}
	if err == nil && report.partial {
		return errPartial
	}
	return err
}

func parseDate(s string) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err == nil && t.Year() == 0 {
		err = errors.New("недопустимый год")
	}
	return t, err
}
