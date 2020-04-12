package logging

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

var DefaultLogger *logrus.Logger

func CreateLogger(fileName string) (*logrus.Logger, error) {
	var logger = logrus.New()
	currentTime := time.Now()
	var fileNameFull string
	i := 0
	for {
		fileNameFull = fmt.Sprintf("%s[%s]-%d.log", fileName, currentTime.Format("01-02-2006"), 0)
		if _, err := os.Stat(fileNameFull); os.IsNotExist(err) {
			break
		}
		i++
	}
	file, err := os.OpenFile(fileNameFull, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return nil, errors.New("Unable to open file: " + fileNameFull)
	}
	mw := io.MultiWriter(os.Stdout, file)
	logger.SetOutput(mw)
	return logger, nil
}

func CloseLogger(logger *logrus.Logger) error {
	return logger.Writer().Close()
}
