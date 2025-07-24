package retry

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
)

func TestTimesWaitTimeout(t *testing.T) {
	model := Times(5).Wait(2 * time.Second).Timeout(3 * time.Second)

	if model.retry != 5 {
		t.Errorf("expected retry=5, got %d", model.retry)
	}
	if model.waitTime != 2*time.Second {
		t.Errorf("expected waitTime=2s, got %s", model.waitTime)
	}
	if model.timeout != 3*time.Second {
		t.Errorf("expected timeout=3s, got %s", model.timeout)
	}
}

func TestTry_ActionSucceedsImmediately(t *testing.T) {
	model := Times(3).Wait(0)

	calls := 0
	action := func(attempt uint) error {
		calls++
		return nil
	}

	err := model.Try(action)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestTry_ActionFailsThenSucceeds(t *testing.T) {
	model := Times(3).Wait(0)

	calls := 0
	action := func(attempt uint) error {
		calls++
		if attempt < 1 {
			return errors.New("fail")
		}
		return nil
	}

	err := model.Try(action)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestTry_ActionAlwaysFails(t *testing.T) {
	model := Times(3).Wait(0)

	action := func(attempt uint) error {
		return errors.New("fail")
	}

	err := model.Try(action)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestTry_BreakOnTerminatedContainer(t *testing.T) {
	model := Times(5).Wait(0)

	terminatedErr := fmt.Errorf("container is in terminated state")
	calls := 0

	action := func(attempt uint) error {
		calls++
		return terminatedErr
	}

	err := model.Try(action)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (break on terminated), got %d", calls)
	}
}

func TestTryWithTimeout_ExceedsTimeout(t *testing.T) {
	model := Times(3).Timeout(10 * time.Millisecond).Wait(0)

	action := func(attempt uint) error {
		time.Sleep(20 * time.Millisecond)
		return nil
	}

	err := model.TryWithTimeout(action)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}

	var cerr cerrors.Error
	if !errors.As(err, &cerr) || cerr.ErrorCode != cerrors.ErrorTypeTimeout {
		t.Errorf("expected timeout cerror, got %v", err)
	}
}

func TestTryWithTimeout_SucceedsWithinTimeout(t *testing.T) {
	model := Times(3).Timeout(50 * time.Millisecond).Wait(0)

	called := 0
	action := func(attempt uint) error {
		called++
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	err := model.TryWithTimeout(action)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if called != 1 {
		t.Errorf("expected 1 call, got %d", called)
	}
}

func TestTry_NilAction(t *testing.T) {
	model := Times(2)
	err := model.Try(nil)
	if err == nil {
		t.Error("expected error for nil action, got nil")
	}
}

func TestTryWithTimeout_NilAction(t *testing.T) {
	model := Times(2)
	err := model.TryWithTimeout(nil)
	if err == nil {
		t.Error("expected error for nil action, got nil")
	}
}
