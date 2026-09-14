package ingestsched

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestDockerLaunchBoundsCrawlAndPreservesShard(t *testing.T) {
	var args []string
	l := DockerLauncher{Image: "freehire:test", Network: "freehire_default", Prefix: "freehire", Environment: []string{"DATABASE_URL"}, exec: func(_ context.Context, a ...string) ([]byte, error) { args = a; return nil, nil }}
	if err := l.Launch(context.Background(), Run{Provider: "greenhouse", Shard: 2, Shards: 4, RunTimeout: DefaultRunTimeout}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--memory=2g", "--cpus=1", "--entrypoint=/usr/bin/timeout", "--kill-after=30s", "3000", "--shard=2/4", "DATABASE_URL", "freehire-ingest-greenhouse-2"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %s: %v", want, args)
		}
	}
	if err := l.Launch(context.Background(), Run{Provider: "--privileged"}); err == nil {
		t.Fatal("unsafe provider accepted")
	}
}

func TestDockerReapsExitStatusAndRetainsRunningCrawls(t *testing.T) {
	for _, tt := range []struct {
		state string
		done  bool
		code  int
	}{
		{`{"Status":"running","ExitCode":0}`, false, 0},
		{`{"Status":"exited","ExitCode":0}`, true, 0},
		{`{"Status":"exited","ExitCode":124}`, true, 124},
		{`{"Status":"exited","ExitCode":137,"OOMKilled":true}`, true, 137},
	} {
		removed := false
		l := DockerLauncher{Prefix: "freehire", exec: func(_ context.Context, args ...string) ([]byte, error) {
			if args[0] == "rm" {
				removed = true
				return nil, nil
			}
			return []byte(tt.state), nil
		}}
		o, err := l.Finished(context.Background(), Run{Provider: "greenhouse", Shard: 1})
		if err != nil || o.Done != tt.done || o.ExitCode != tt.code || removed != tt.done {
			t.Fatalf("state=%s outcome=%+v removed=%v error=%v", tt.state, o, removed, err)
		}
	}
}

func TestDockerDaemonFailureIsNotReportedAsCompleted(t *testing.T) {
	l := DockerLauncher{Prefix: "freehire", exec: func(context.Context, ...string) ([]byte, error) {
		return []byte("Cannot connect to the Docker daemon"), errors.New("exit 1")
	}}
	if _, err := l.Finished(context.Background(), Run{Provider: "greenhouse", Shard: 1}); err == nil {
		t.Fatal("daemon failure accepted")
	}
	l.exec = func(context.Context, ...string) ([]byte, error) {
		return []byte("Error: No such object: freehire-ingest-greenhouse-1"), errors.New("exit 1")
	}
	o, err := l.Finished(context.Background(), Run{Provider: "greenhouse", Shard: 1})
	if err != nil || !o.Done || o.ExitCode == 0 || !strings.Contains(o.Detail, "disappeared") {
		t.Fatalf("missing container reported as success: %+v %v", o, err)
	}
}

func TestDockerLaunchRecoversAmbiguousStart(t *testing.T) {
	for _, running := range []bool{true, false} {
		removed := false
		l := DockerLauncher{Image: "freehire:test", Network: "crawl", Prefix: "freehire", exec: func(_ context.Context, args ...string) ([]byte, error) {
			switch args[0] {
			case "run":
				return []byte("connection lost"), errors.New("exit 1")
			case "inspect":
				if running {
					return []byte("true\n"), nil
				}
				return []byte("false\n"), nil
			case "rm":
				removed = true
				if slices.Contains(args, "--force") {
					t.Fatal("must never force-remove a crawl")
				}
				return nil, nil
			default:
				t.Fatalf("unexpected command: %v", args)
				return nil, nil
			}
		}}
		err := l.Launch(context.Background(), Run{Provider: "greenhouse", Shard: 1, RunTimeout: DefaultRunTimeout})
		if (err == nil) != running || removed == running {
			t.Fatalf("running=%v removed=%v error=%v", running, removed, err)
		}
	}
}
