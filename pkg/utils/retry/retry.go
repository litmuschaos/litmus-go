package retry

import (
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"time"

)

// Action defines the prototype of action function, function as a value
type Action func(attempt uint) error

// Model defines the schema, contains all the attributes need for retry
type Model struct {
	retry    uint
	waitTime time.Duration
	timeout  time.Duration
}

// Times is used to define the retry count
// it will run if the instance of model is not present before
func Times(retry uint) *Model {
	model := Model{}
	return model.Times(retry)
}

// Times is used to define the retry count
// it will run if the instance of model is already present
func (model *Model) Times(retry uint) *Model {
	model.retry = retry
	return model
}

// Wait is used to define the wait duration after each iteration of retry
// it will run if the instance of model is not present before
func Wait(waitTime time.Duration) *Model {
	model := Model{}
	return model.Wait(waitTime)
}

// Wait is used to define the wait duration after each iteration of retry
// it will run if the instance of model is already present
func (model *Model) Wait(waitTime time.Duration) *Model {
	model.waitTime = waitTime
	return model
}

// Timeout is used to define the timeout duration for each iteration of retry
// it will run if the instance of model is not present before
func Timeout(timeout time.Duration) *Model {
	model := Model{}
	return model.Timeout(timeout)
}

// Timeout is used to define the timeout duration for each iteration of retry
// it will run if the instance of model is already present
func (model *Model) Timeout(timeout time.Duration) *Model {
	model.timeout = timeout
	return model
}

// Try is used to run a action with retries and some delay after each iteration
func (model Model) Try(action Action) error {
	if action == nil {
		return fmt.Errorf("no action specified")
	}

	var err error
	for attempt := uint(0); (attempt == 0 || err != nil) && attempt < model.retry; attempt++ {
		err = action(attempt)
		if model.waitTime > 0 {
			time.Sleep(model.waitTime)
		}
		// Match based on error string to support testability and avoid fragile pointer comparison
		if err != nil && err.Error() == "container is in terminated state" {
			break
		}
		
	}

	return err
}

// TryWithTimeout is used to run an action with retries
// for each iteration of attempt there will be some timeout
func (model Model) TryWithTimeout(action Action) error {
	if action == nil {
		return fmt.Errorf("no action specified")
	}
	var err error
	err = nil
	for attempt := uint(0); (attempt == 0 || err != nil) && attempt < model.retry; {
		startTime := time.Now().UnixMilli()
		err = action(attempt)
		if err == nil && time.Now().UnixMilli()-startTime >= model.timeout.Milliseconds() {
			err = cerrors.Error{
				ErrorCode: cerrors.ErrorTypeTimeout,
				Reason:    "action timeout",
			}
		}
		attempt++
		if model.waitTime > 0 && attempt < model.retry {
			time.Sleep(model.waitTime)
		}
	}

	return err
}
