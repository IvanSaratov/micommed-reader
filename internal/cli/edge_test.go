package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestInputFailureContracts(t *testing.T) {
	dir := t.TempDir()
	damaged := put(t, dir, "20010203", fixture(t, "truncated"))
	raw := invoke(t, 3, "export", "json", damaged)
	if strings.Contains(raw, `"decoder_version":""`) {
		t.Error("пропала версия декодера")
	}
	if err := os.Chmod(damaged, 0); err != nil {
		t.Fatal(err)
	}
	if f, e := os.Open(damaged); e == nil {
		f.Close()
	} else {
		invoke(t, 1, "inspect", damaged)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err == nil {
		invoke(t, 1, "inspect", link+string(os.PathSeparator))
	}
	body := []byte("пример служебного файла")
	z := zipInput(t, member{"ID.txt", body})
	b, err := os.ReadFile(z)
	if err != nil {
		t.Fatal(err)
	}
	b[bytes.Index(b, body)] ^= 1
	invoke(t, 1, "inspect", put(t, t.TempDir(), "notes.zip", b))
}

func TestCSVDirectoryPublication(t *testing.T) {
	file := put(t, t.TempDir(), "20010203", fixture(t, "clock"))
	dest := filepath.Join(t.TempDir(), "tables")
	invoke(t, 0, "export", "csv", "-o", dest+string(os.PathSeparator), file)
	r := report{}
	dest = filepath.Join(t.TempDir(), "concurrent")
	var wg sync.WaitGroup
	outcomes := make(chan error, 8)
	for range 8 {
		wg.Go(func() { outcomes <- r.writeCSV(dest) })
	}
	wg.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("опубликовано %d результатов", successes)
	}
}

func TestFlagErrorsAreLocalized(t *testing.T) {
	for _, args := range [][]string{{"inspect", "--unknown"}, {"inspect", "--date"}} {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), append([]string{"healdata"}, args...), &out, &stderr)
		if code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), args[1]) || strings.Contains(stderr.String(), "flag ") {
			t.Fatalf("аргументы %s: %s", fmt.Sprint(args), stderr.String())
		}
	}
}
