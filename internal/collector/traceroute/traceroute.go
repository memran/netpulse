package traceroute

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
)

type Runner struct {
	log     *logger.Logger
	st      *state.AppState
	maxHops int
	probes  int
}

func NewRunner(log *logger.Logger, st *state.AppState, maxHops, probes int) *Runner {
	return &Runner{
		log:     log.WithComponent("collector/traceroute"),
		st:      st,
		maxHops: maxHops,
		probes:  probes,
	}
}

func (r *Runner) Run(ctx context.Context, target string) {
	r.st.SetTraceroute(state.TracerouteResult{Running: true})
	r.log.Infof("traceroute to %s started", target)

	hops, err := r.execute(ctx, target)

	result := state.TracerouteResult{
		Target:      target,
		Hops:        hops,
		Running:     false,
		CompletedAt: time.Now(),
	}
	if err != nil {
		result.Error = err.Error()
		r.log.Warnf("traceroute failed: %v", err)
	}

	r.st.SetTraceroute(result)
	r.log.Infof("traceroute to %s completed: %d hops", target, len(hops))
}

func (r *Runner) execute(ctx context.Context, target string) ([]state.TracerouteHop, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin", "linux":
		args := []string{
			"-n",
			"-I",
			fmt.Sprintf("-q%d", r.probes),
			fmt.Sprintf("-m%d", r.maxHops),
			target,
		}
		cmd = exec.CommandContext(ctx, "traceroute", args...)
	case "windows":
		args := []string{
			"-d",
			fmt.Sprintf("-h%d", r.maxHops),
			target,
		}
		cmd = exec.CommandContext(ctx, "tracert", args...)
	default:
		args := []string{
			"-n",
			"-I",
			fmt.Sprintf("-q%d", r.probes),
			fmt.Sprintf("-m%d", r.maxHops),
			target,
		}
		cmd = exec.CommandContext(ctx, "traceroute", args...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("traceroute exited with %d: %s", exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("traceroute failed: %w", err)
	}

	return parseOutput(string(output), runtime.GOOS), nil
}

func parseOutput(output string, goos string) []state.TracerouteHop {
	var hops []state.TracerouteHop
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		hopNum, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		hop := state.TracerouteHop{Hop: hopNum}

		switch goos {
		case "windows":
			var rtts []float64
			for _, f := range fields[1:] {
				switch {
				case f == "*":
					rtts = append(rtts, 0)
				case f == "<1":
					rtts = append(rtts, 1)
				case f == "ms" || f == "Request" || f == "timed" || f == "out.":
					continue
				default:
					if v, err := strconv.ParseFloat(f, 64); err == nil {
						rtts = append(rtts, v)
					} else if ip := net.ParseIP(f); ip != nil {
						hop.IP = f
					}
				}
			}
			hop.RTTs = rtts
		default:
			hop.IP = fields[1]
			for _, f := range fields[2:] {
				if f == "*" {
					hop.RTTs = append(hop.RTTs, 0)
				} else if v, err := strconv.ParseFloat(f, 64); err == nil {
					hop.RTTs = append(hop.RTTs, v)
				}
			}
		}

		hops = append(hops, hop)
	}

	return hops
}
