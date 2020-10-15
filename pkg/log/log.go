package log

import (
	logrus "github.com/sirupsen/logrus"
)

//Fatalf Logs first and then calls `logger.Exit(1)`
// logging level is set to Panic.
func Fatalf(msg string, err ...interface{}) {
	logrus.WithFields(logrus.Fields{}).Fatalf(msg, err...)
}

//Fatal Logs first and then calls `logger.Exit(1)`
// logging level is set to Panic.
func Fatal(msg string) {
	logrus.WithFields(logrus.Fields{}).Fatal(msg)
}

//Infof log the General operational entries about what's going on inside the application
func Infof(msg string, val ...interface{}) {
	logrus.WithFields(logrus.Fields{}).Infof(msg, val...)
}

//Info log the General operational entries about what's going on inside the application
func Info(msg string) {
	logrus.WithFields(logrus.Fields{}).Infof(msg)
}

// InfoWithValues log the General operational entries about what's going on inside the application
// It also print the extra key values pairs
func InfoWithValues(msg string, val map[string]interface{}) {
	logrus.WithFields(val).Info(msg)
}

// ErrorWithValues log the Error entries happening inside the code
// It also print the extra key values pairs
func ErrorWithValues(msg string, val map[string]interface{}) {
	logrus.WithFields(val).Error(msg)
}

//Warn log the Non-critical entries that deserve eyes.
func Warn(msg string) {
	logrus.WithFields(logrus.Fields{}).Warn(msg)
}

//Warnf log the Non-critical entries that deserve eyes.
func Warnf(msg string, val ...interface{}) {
	logrus.WithFields(logrus.Fields{}).Warnf(msg, val...)
}

//Errorf used for errors that should definitely be noted.
// Commonly used for hooks to send errors to an error tracking service.
func Errorf(msg string, err ...interface{}) {
	logrus.WithFields(logrus.Fields{}).Errorf(msg, err...)
}

//Error used for errors that should definitely be noted.
// Commonly used for hooks to send errors to an error tracking service
func Error(msg string) {
	logrus.WithFields(logrus.Fields{}).Error(msg)
}
