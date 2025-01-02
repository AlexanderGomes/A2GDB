package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

func InitLogger() {
	Log = logrus.New()
	logFile, err := os.OpenFile("./logger/app.json", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		logFile, _ = os.OpenFile("app.json", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	}

	Log.SetOutput(logFile)
	Log.SetFormatter(&logrus.JSONFormatter{
		DisableTimestamp: true,
	})

	Log.SetLevel(logrus.InfoLevel)
}
