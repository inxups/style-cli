package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

func runStats(ctx context.Context, args []string, out, errOut io.Writer) int {
	return runStatsAt(ctx, args, out, errOut, time.Now())
}

func runStatsAt(ctx context.Context, args []string, out, errOut io.Writer, now time.Time) int {
	return runStatsAtHome(ctx, args, out, errOut, now, "")
}

func runStatsAtHome(ctx context.Context, args []string, out, errOut io.Writer, now time.Time, home string) int {
	if helpRequested(args) {
		fmt.Fprintln(out, statsCommand.usage())
		return ExitOK
	}
	parsed, err := statsCommand.parse(args)
	if err != nil {
		return statsCommand.usageError(errOut, err)
	}
	request, jsonOutput := StatsRequest{By: parsed.String("--by"), Since: parsed.String("--since")}, parsed.Bool("--json")
	if home != "" {
		request.Home = home
	}
	result, err := statsAt(ctx, request, now)
	if err != nil {
		fmt.Fprintf(errOut, "style stats: %v\n", err)
		return ExitInvalid
	}
	var report bytes.Buffer
	if jsonOutput {
		err = encodeJSON(&report, result)
	} else {
		writeStatsText(&report, result)
	}
	if err == nil {
		_, err = io.Copy(out, &report)
	}
	if err != nil {
		fmt.Fprintln(errOut, "style stats: cannot write report")
		return ExitInvalid
	}
	return ExitOK
}

func writeStatsText(out io.Writer, result StatsResult) {
	fmt.Fprintf(out, "Window: %s .. %s\n", result.Window.Since.Format(time.RFC3339Nano), result.Window.Until.Format(time.RFC3339Nano))
	fmt.Fprintln(out, "available runtime records; estimated tokens; calibration pending")
	c := result.Totals
	fmt.Fprintf(out, "Totals: outputs=%d blocks=%d dedup=%d cap_drops=%d estimated_tokens=%d unestimated=%d\n", c.Outputs, c.Blocks, c.Dedup, c.CapDrops, c.EstimatedTokens, c.Unestimated)
	if len(result.Rows) == 0 {
		fmt.Fprintln(out, "No runtime records in window.")
	} else {
		table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintf(table, "PROVIDER\t%s\tOUTPUTS\tBLOCKS\tDEDUP\tCAP_DROPS\tEST_TOKENS\tUNESTIMATED\n", strings.ToUpper(result.By))
		for _, row := range result.Rows {
			fmt.Fprintf(table, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\n", statsIdentifier(row.Provider), statsIdentifier(row.Rule+row.Session), row.Outputs, row.Blocks, row.Dedup, row.CapDrops, row.EstimatedTokens, row.Unestimated)
		}
		table.Flush()
	}
	if result.Quality.Partial {
		q := result.Quality
		fmt.Fprintf(out, "Quality: partial=true invalid_lines=%d oversized_lines=%d skipped_symlinks=%d\n", q.InvalidLines, q.OversizedLines, q.SkippedSymlinks)
	}
}

func statsIdentifier(value string) string {
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}
