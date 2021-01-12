// Copyright 2019 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package mtail_test

import (
	"expvar"
	"os"
	"path"
	"testing"

	"github.com/golang/glog"
	"github.com/google/mtail/internal/mtail"
	"github.com/google/mtail/internal/testutil"
)

func TestGlobBeforeStart(t *testing.T) {
	testutil.SkipIfShort(t)

	workdir, rmWorkdir := testutil.TestTempDir(t)
	defer rmWorkdir()

	globTests := []struct {
		name     string
		expected bool
	}{
		{
			path.Join(workdir, "log1"),
			true,
		},
		{
			path.Join(workdir, "log2"),
			true,
		},
		{
			path.Join(workdir, "1log"),
			false,
		},
	}
	var count int64
	for _, tt := range globTests {
		log := testutil.TestOpenFile(t, tt.name)
		defer log.Close()
		if tt.expected {
			count++
		}
		testutil.WriteString(t, log, "\n")
	}
	m, stopM := mtail.TestStartServer(t, 0, 0, mtail.LogPathPatterns(path.Join(workdir, "log*")))
	stopM()

	if r := m.GetExpvar("log_count"); r.(*expvar.Int).Value() != int64(count) {
		t.Errorf("Expecting log count of %d, received %d", count, r)
	}
}

func TestGlobAfterStart(t *testing.T) {
	testutil.SkipIfShort(t)

	workdir, rmWorkdir := testutil.TestTempDir(t)
	defer rmWorkdir()

	globTests := []struct {
		name     string
		expected bool
	}{
		{
			path.Join(workdir, "log1"),
			true,
		},
		{
			path.Join(workdir, "log2"),
			true,
		},
		{
			path.Join(workdir, "1log"),
			false,
		},
	}
	m, stopM := mtail.TestStartServer(t, 0, 0, mtail.LogPathPatterns(path.Join(workdir, "log*")))
	defer stopM()

	m.PollWatched(0) // Force sync to EOF

	var count int64
	for _, tt := range globTests {
		if tt.expected {
			count++
		}
	}
	logCountCheck := m.ExpectExpvarDeltaWithDeadline("log_count", count)
	for _, tt := range globTests {
		log := testutil.TestOpenFile(t, tt.name)
		m.PollWatched(0) // Force sync to EOF
		defer log.Close()
	}
	// m.PollWatched(2)
	logCountCheck()
}

func TestGlobIgnoreFolder(t *testing.T) {
	testutil.SkipIfShort(t)

	workdir, rmWorkdir := testutil.TestTempDir(t)
	defer rmWorkdir()

	globTests := []struct {
		name     string
		isFolder bool
		expected bool
	}{
		{
			path.Join(workdir, "log1"),
			false,
			true,
		},
		{
			path.Join(workdir, "logarchive"),
			true,
			false,
		},
		{
			path.Join(workdir, "log2.gz"),
			false,
			false,
		},
	}
	var count int64
	for _, tt := range globTests {
		var err error
		var log *os.File

		if tt.isFolder {
			err = os.Mkdir(tt.name, 0700)
			testutil.FatalIfErr(t, err)
			continue
		} else {
			log, err = os.Create(tt.name)
		}

		if !tt.isFolder && tt.expected {
			count++
		}
		defer log.Close()
		testutil.FatalIfErr(t, err)
		testutil.WriteString(t, log, "\n")
	}

	m, stopM := mtail.TestStartServer(t, 0, 0, mtail.LogPathPatterns(path.Join(workdir, "log*")), mtail.IgnoreRegexPattern("\\.gz"))

	stopM()

	if r := m.GetExpvar("log_count"); r.(*expvar.Int).Value() != count {
		t.Errorf("Expecting log count of %d, received %v", count, r)
	}
}

func TestFilenameRegexIgnore(t *testing.T) {
	testutil.SkipIfShort(t)

	workdir, rmWorkdir := testutil.TestTempDir(t)
	defer rmWorkdir()

	globTests := []struct {
		name     string
		expected bool
	}{
		{
			path.Join(workdir, "log1"),
			true,
		},
		{
			path.Join(workdir, "log1.gz"),
			false,
		},
		{
			path.Join(workdir, "log2gz"),
			true,
		},
	}
	var count int64
	for _, tt := range globTests {
		log, err := os.Create(tt.name)
		testutil.FatalIfErr(t, err)
		defer log.Close()
		if tt.expected {
			count++
		}
		testutil.WriteString(t, log, "\n")
	}

	m, stopM := mtail.TestStartServer(t, 0, 0, mtail.LogPathPatterns(path.Join(workdir, "log*")), mtail.IgnoreRegexPattern("\\.gz"))

	stopM()

	if r := m.GetExpvar("log_count"); r.(*expvar.Int).Value() != count {
		t.Errorf("Log count not matching, expected: %d received: %v", count, r)
	}
}

func TestGlobRelativeAfterStart(t *testing.T) {
	testutil.SkipIfShort(t)
	tmpDir, rmTmpDir := testutil.TestTempDir(t)
	defer rmTmpDir()

	logDir := path.Join(tmpDir, "logs")
	progDir := path.Join(tmpDir, "progs")
	err := os.Mkdir(logDir, 0700)
	testutil.FatalIfErr(t, err)
	err = os.Mkdir(progDir, 0700)
	testutil.FatalIfErr(t, err)

	// Move to logdir to make relative paths
	defer testutil.TestChdir(t, logDir)()

	m, stopM := mtail.TestStartServer(t, 0, 1, mtail.ProgramPath(progDir), mtail.LogPathPatterns("log.*"))
	defer stopM()

	{
		logCountCheck := m.ExpectExpvarDeltaWithDeadline("log_count", 1)

		logFile := path.Join(logDir, "log.1.txt")
		f := testutil.TestOpenFile(t, logFile)
		m.PollWatched(1) // Force sync to EOF
		testutil.WriteString(t, f, "line 1\n")
		m.PollWatched(1)

		logCountCheck()
	}

	{

		logCountCheck := m.ExpectExpvarDeltaWithDeadline("log_count", 1)

		logFile := path.Join(logDir, "log.2.txt")
		f := testutil.TestOpenFile(t, logFile)
		m.PollWatched(2)
		testutil.WriteString(t, f, "line 1\n")
		m.PollWatched(2)

		logCountCheck()
	}
	{
		logCountCheck := m.ExpectExpvarDeltaWithDeadline("log_count", 0)

		logFile := path.Join(logDir, "log.2.txt")
		f := testutil.TestOpenFile(t, logFile)
		m.PollWatched(2)
		testutil.WriteString(t, f, "line 2\n")
		m.PollWatched(2)

		logCountCheck()
	}

	glog.Infof("end")
}
