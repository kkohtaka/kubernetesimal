package controllers

import (
	"errors"
	"fmt"
	"time"
)

type RequeueError struct {
	msg string
	err error

	delay time.Duration
}

var _ error = (*RequeueError)(nil)

func NewRequeueError(msg string) *RequeueError {
	return &RequeueError{
		msg: msg,
	}
}

func (re *RequeueError) Error() string {
	if re.err == nil {
		return re.msg
	}
	return fmt.Sprintf("%s: %s", re.msg, re.err)
}

func (re *RequeueError) Unwrap() error {
	return re.err
}

func (re *RequeueError) WithDelay(delay time.Duration) *RequeueError {
	newErr := *re
	newErr.delay = delay
	return &newErr
}

func (re *RequeueError) Wrap(err error) *RequeueError {
	newErr := *re
	newErr.err = err
	return &newErr
}

func ShouldRequeue(err error) bool {
	var re *RequeueError
	if ok := errors.As(err, &re); !ok {
		return false
	}
	return true
}

func GetDelay(err error) time.Duration {
	var re *RequeueError
	if ok := errors.As(err, &re); ok {
		return re.delay
	}
	return 0
}
