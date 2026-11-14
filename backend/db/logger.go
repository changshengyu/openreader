package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

// NewLogger returns the GORM logger Open installs, writing to writer. It
// prints what GORM's default logger prints for the same Config, except a
// statement that failed only because its own context ended: the client went
// away or the caller's deadline passed. The caller still gets that error.
func NewLogger(writer gormlogger.Writer) gormlogger.Interface {
	config := gormlogger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: false,
		Colorful:                  true,
		ParameterizedQueries:      true,
	}
	logger := &statementLogger{
		Interface:    gormlogger.New(writer, config),
		writer:       writer,
		config:       config,
		traceStr:     "%s\n[%.3fms] [rows:%v] %s",
		traceWarnStr: "%s %s\n[%.3fms] [rows:%v] %s",
		traceErrStr:  "%s %s\n[%.3fms] [rows:%v] %s",
	}
	if config.Colorful {
		logger.traceStr = gormlogger.Green + "%s\n" + gormlogger.Reset + gormlogger.Yellow + "[%.3fms] " + gormlogger.BlueBold + "[rows:%v]" + gormlogger.Reset + " %s"
		logger.traceWarnStr = gormlogger.Green + "%s " + gormlogger.Yellow + "%s\n" + gormlogger.Reset + gormlogger.RedBold + "[%.3fms] " + gormlogger.Yellow + "[rows:%v]" + gormlogger.Magenta + " %s" + gormlogger.Reset
		logger.traceErrStr = gormlogger.RedBold + "%s " + gormlogger.MagentaBold + "%s\n" + gormlogger.Reset + gormlogger.Yellow + "[%.3fms] " + gormlogger.BlueBold + "[rows:%v]" + gormlogger.Reset + " %s"
	}
	return logger
}

// statementLogger embeds GORM's logger for Info, Warn and Error; promoted
// methods keep GORM's caller attribution. Trace is written out here, with
// GORM's formats, because utils.FileWithLineNum reports the first caller
// outside GORM: a Trace that called GORM's Trace would put this file on
// every line instead of the code that ran the statement.
type statementLogger struct {
	gormlogger.Interface
	writer                              gormlogger.Writer
	config                              gormlogger.Config
	traceStr, traceWarnStr, traceErrStr string
}

func (l *statementLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	next := *l
	next.Interface = l.Interface.LogMode(level)
	next.config.LogLevel = level
	return &next
}

// ParamsFilter keeps ParameterizedQueries working: GORM only asks a logger
// that implements it, and an embedded interface does not carry it over.
func (l *statementLogger) ParamsFilter(_ context.Context, sql string, params ...interface{}) (string, []interface{}) {
	if l.config.ParameterizedQueries {
		return sql, nil
	}
	return sql, params
}

func (l *statementLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.config.LogLevel <= gormlogger.Silent || statementContextEnded(ctx, err) {
		return
	}
	elapsed := time.Since(begin)
	milliseconds := float64(elapsed.Nanoseconds()) / 1e6
	switch {
	case err != nil && l.config.LogLevel >= gormlogger.Error && (!errors.Is(err, gormlogger.ErrRecordNotFound) || !l.config.IgnoreRecordNotFoundError):
		sql, rows := fc()
		l.writer.Printf(l.traceErrStr, utils.FileWithLineNum(), err, milliseconds, traceRows(rows), sql)
	case elapsed > l.config.SlowThreshold && l.config.SlowThreshold != 0 && l.config.LogLevel >= gormlogger.Warn:
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.config.SlowThreshold)
		l.writer.Printf(l.traceWarnStr, utils.FileWithLineNum(), slowLog, milliseconds, traceRows(rows), sql)
	case l.config.LogLevel == gormlogger.Info:
		sql, rows := fc()
		l.writer.Printf(l.traceStr, utils.FileWithLineNum(), milliseconds, traceRows(rows), sql)
	}
}

// statementContextEnded reports whether err comes from the statement's own
// context ending. GORM joins a failed statement's error and the later
// rollback error as "%v; %w", which leaves only the rollback error in the
// chain: sql.ErrTxDone, because database/sql already rolled the transaction
// back when the context ended.
func statementContextEnded(ctx context.Context, err error) bool {
	if err == nil || ctx == nil || ctx.Err() == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, sql.ErrTxDone)
}

func traceRows(rows int64) interface{} {
	if rows == -1 {
		return "-"
	}
	return rows
}
