package lib

// Command is a type for representing executable commands on a shell
type Command string

// Executor is an interface to represent exec() providers.
type Executor interface {
	Execute(command Command) error
}

// Shell is an abstraction to lift errors from Executor(s). It wraps
// error values to provide safe successive command execution.
type Shell struct {
	executor Executor
	err      error
}

// Run safely invokes the Executor
func (shell *Shell) Run(command Command) {
	if shell.err != nil {
		return
	}

	shell.err = shell.executor.Execute(command)
}

// Script represents a sequence of commands
type Script []Command

// RunOn runs the script on the given shell
func (script Script) RunOn(shell Shell) error {
	for _, command := range script {
		shell.Run(command)
	}

	return shell.err
}
