package util

import "github.com/sirupsen/logrus"

func ContinueOrFatal(err error) {
	if err != nil {
		logrus.Fatal(err)
	}
}
