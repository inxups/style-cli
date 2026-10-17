package source_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/inxups/style-cli/internal/source"
)

func TestM3A17WindowsDirectProcessCancellation(t *testing.T) {
	if os.Getenv("STYLE_WINDOWS_CANCEL_HELPER") == "1" {
		time.Sleep(time.Minute)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = (source.ExecRunner{}).Run(ctx, source.Command{
		Program: executable, Args: []string{"-test.run=^TestM3A17WindowsDirectProcessCancellation$"},
		Env: append(os.Environ(), "STYLE_WINDOWS_CANCEL_HELPER=1"),
	})
	if err == nil || ctx.Err() == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("cancellation: elapsed=%v err=%v context=%v", time.Since(start), err, ctx.Err())
	}
}
