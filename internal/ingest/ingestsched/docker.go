package ingestsched

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DockerLauncher uses the same database claims as SystemdLauncher, but each crawl
// lives in a bounded sibling container. Containers survive scheduler restarts and
// retain their exit status until the next tick records it.
type DockerLauncher struct {
	Image, Network, Prefix string
	// Environment contains variable names, not secrets in command arguments.
	Environment []string
	exec        func(context.Context, ...string) ([]byte, error)
}

func (l DockerLauncher) command(ctx context.Context, args ...string) ([]byte, error) {
	if l.exec != nil {
		return l.exec(ctx, args...)
	}
	return exec.CommandContext(ctx, "docker", args...).CombinedOutput()
}

func (l DockerLauncher) name(run Run) (string, error) {
	if err := ValidateProviderKey(run.Provider); err != nil {
		return "", err
	}
	if l.Prefix == "" || strings.ContainsAny(l.Prefix, " /\\\t\n") || strings.HasPrefix(l.Prefix, "-") {
		return "", fmt.Errorf("a valid Docker container prefix is required")
	}
	return l.Prefix + "-ingest-" + run.Provider + "-" + strconv.Itoa(run.Shard), nil
}

func (l DockerLauncher) Launch(ctx context.Context, run Run) error {
	name, err := l.name(run)
	if err != nil {
		return err
	}
	if l.Image == "" || l.Network == "" || run.RunTimeout.Seconds() < 1 {
		return fmt.Errorf("Docker image, network and positive crawl timeout are required")
	}
	args := []string{"run", "--detach", "--init", "--name", name,
		"--network", l.Network, "--restart=no", "--cpus=1", "--memory=2g",
		"--log-opt=max-size=10m", "--log-opt=max-file=2",
		"--label", "freehire.scheduler=" + l.Prefix,
		"--entrypoint=/usr/bin/timeout"}
	for _, key := range l.Environment {
		args = append(args, "--env", key)
	}
	args = append(args, l.Image, "--signal=TERM", "--kill-after=30s",
		strconv.Itoa(int(run.RunTimeout.Seconds())), "/app/ingest", run.Provider)
	if run.Shards > 1 {
		args = append(args, fmt.Sprintf("--shard=%d/%d", run.Shard, run.Shards))
	}
	if out, err := l.command(ctx, args...); err != nil {
		// A failed CLI request can leave a created container, or lose the response
		// after Docker started it. Adopt a running crawl; remove only stopped
		// leftovers (rm without --force cannot kill a live crawl).
		state, inspectErr := l.command(ctx, "inspect", "--format={{.State.Running}}", name)
		if inspectErr == nil {
			if strings.TrimSpace(string(state)) == "true" {
				return nil
			}
			_, _ = l.command(ctx, "rm", name)
		}
		return fmt.Errorf("start %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (l DockerLauncher) Finished(ctx context.Context, run Run) (Outcome, error) {
	name, err := l.name(run)
	if err != nil {
		return Outcome{}, err
	}
	out, err := l.command(ctx, "inspect", "--format={{json .State}}", name)
	if err != nil {
		if strings.Contains(string(out), "No such object:") || strings.Contains(string(out), "No such container:") {
			// Docker cleanup may remove a stopped container before we reap it. Unlike
			// systemd, disappearance says nothing about whether the crawl succeeded.
			return Outcome{Done: true, ExitCode: 125, Detail: "crawl container disappeared"}, nil
		}
		return Outcome{}, fmt.Errorf("inspect %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	var state struct {
		Status    string
		ExitCode  int
		OOMKilled bool
		Error     string
	}
	if err := json.Unmarshal(out, &state); err != nil {
		return Outcome{}, fmt.Errorf("decode %s state: %w", name, err)
	}
	if state.Status != "exited" && state.Status != "dead" && state.Status != "created" {
		return Outcome{Done: false}, nil
	}
	detail := state.Error
	if state.Status == "created" {
		state.ExitCode = 125
		detail = "crawl container was created but never started"
	}
	if state.OOMKilled {
		detail = "memory limit exceeded"
	} else if state.ExitCode == 124 || state.ExitCode == 137 {
		detail = "crawl timed out or was killed"
	}
	if _, err := l.command(ctx, "rm", name); err != nil {
		// Do not release the claim while its name would block the next launch.
		return Outcome{}, fmt.Errorf("remove completed %s: %w", name, err)
	}
	return Outcome{Done: true, ExitCode: state.ExitCode, Detail: detail}, nil
}
