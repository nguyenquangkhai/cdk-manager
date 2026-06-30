package adapter

import "github.com/nguyenquangkhai/cdk-manager/internal/target"

type Operation string

const (
	OpDeploy  Operation = "deploy"
	OpDestroy Operation = "destroy"
	OpDiff    Operation = "diff"
	OpSynth   Operation = "synth"
)

type Command struct {
	Name string
	Args []string
	Env  map[string]string
	Dir  string
}

type State string

const (
	StatePending State = "pending"
	StateRunning State = "running"
	StateSynth   State = "synth"
	StateDeploy  State = "deploy"
	StateDone    State = "done"
	StateFailed  State = "failed"
)

type Adapter interface {
	Build(t target.Target, op Operation, stacks []string, requireApproval string) Command
	OutputDir(t target.Target) string
	ParseStatus(line string) (State, bool)
}
