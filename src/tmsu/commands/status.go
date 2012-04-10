/*
Copyright 2011-2012 Paul Ruane.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"tmsu/common"
	"tmsu/database"
)

type StatusCommand struct{}

func (StatusCommand) Name() string {
	return "status"
}

func (StatusCommand) Synopsis() string {
	return "List the file tagging status"
}

func (StatusCommand) Description() string {
	return `tmsu status [PATH]...

Shows the status of PATHs (current directory by default).

  --directory    List directory entries instead of contents.

Status codes are:

  T - Tagged
  M - Modified
  ! - Missing
  ? - Untagged
  + - Nested

Nested (+) indicates a directory is not itself tagged but some of the files and
directories within it are.

Note: The 'repair' command can be used to fix problems caused by files that have
been modified or moved on disk.`
}

type StatusReport struct {
	Tagged   []string
	Modified []string
	Missing  []string
	Untagged []string
	Nested   []string
}

func NewReport() *StatusReport {
	return &StatusReport{make([]string, 0, 10), make([]string, 0, 10), make([]string, 0, 10), make([]string, 0, 10), make([]string, 0, 10)}
}

func (command StatusCommand) Exec(args []string) error {
    showDirectory := false

    if len(args) > 0 && args[0] == "--directory" {
        showDirectory = true
        args = args[1:]
    }

    report := NewReport()

    err := command.status(args, report, showDirectory)
    if err != nil {
        return err
    }

    for _, path := range report.Tagged {
        fmt.Println("T", path)
    }

	for _, path := range report.Modified {
        fmt.Println("M", path)
	}

    for _, path := range report.Nested {
        fmt.Println("+", path)
    }

	for _, path := range report.Missing {
        fmt.Println("!", path)
	}

	for _, path := range report.Untagged {
	    fmt.Println("?", path)
    }

	return nil
}

func (command StatusCommand) status(paths []string, report *StatusReport, showDirectory bool) error {
    if len(paths) == 0 {
        paths = []string{"."}
    }

    db, err := database.Open()
    if err != nil {
        return err
    }

    for _, path := range paths {
        absPath, err := filepath.Abs(path)
        if err != nil {
            return err
        }

        if !showDirectory && isDir(absPath) {
            status, err := command.getStatus(absPath, db)
            if err != nil {
                return err
            }

            switch status {
            case TAGGED, MODIFIED, MISSING:
                err = command.addToReport(absPath, status, report)
                if err != nil {
                    return err
                }
            case UNTAGGED, NESTED:
                dir, err := os.Open(absPath)
                if err != nil {
                    return err
                }
                defer dir.Close()

                entryNames, err := dir.Readdirnames(0)
                for _, entryName := range entryNames {
                    entryPath := filepath.Join(absPath, entryName)

                    status, err := command.getStatus(entryPath, db)
                    if err != nil {
                        return err
                    }

                    err = command.addToReport(entryPath, status, report)
                    if err != nil {
                        return err
                    }
                }

                files, err := db.FilesByDirectory(absPath)
                for _, file := range files {
                    status, err := command.getStatus(file.Path(), db)
                    if err != nil {
                        return err
                    }

                    if status == MISSING {
                        command.addToReport(file.Path(), status, report)
                    }
                }
            default:
                panic("Unsupported state " + string(status))
            }

        } else {
            status, err := command.getStatus(absPath, db)
            if err != nil {
                return err
            }

            if status == MISSING {
                command.addToReport(absPath, status, report)
            }
        }
    }

    return nil
}

func (command StatusCommand) addToReport(path string, status Status, report *StatusReport) error {
    relPath := common.MakeRelative(path)

    switch status {
    case UNTAGGED:
        report.Untagged = append(report.Untagged, relPath)
    case TAGGED:
        report.Tagged = append(report.Tagged, relPath)
    case MODIFIED:
        report.Modified = append(report.Modified, relPath)
    case MISSING:
        report.Missing = append(report.Missing, relPath)
    case NESTED:
        report.Nested = append(report.Nested, relPath)
    default:
        panic("Unsupported status " + string(status))
    }

    return nil
}

func (command StatusCommand) getStatus(path string, db *database.Database) (Status, error) {
    entry, err := db.FileByPath(path)
    if err != nil {
        return 0, err
    }
    if entry != nil {
        return command.getTaggedPathStatus(entry)
    } else {
        return command.getUntaggedPathStatus(path, db)
    }

    return 0, nil
}

func (command StatusCommand) getTaggedPathStatus(entry *database.File) (Status, error) {
    info, err := os.Stat(entry.Path())
    if err != nil {
        switch {
        case os.IsNotExist(err):
            return MISSING, nil
        default:
            return 0, err
        }
    }

    if entry.ModTimestamp.Unix() == info.ModTime().Unix() {
        return TAGGED, nil
    }

    return MODIFIED, nil
}

func (command StatusCommand) getUntaggedPathStatus(path string, db *database.Database) (Status, error) {
    if common.IsDir(path) {
        dir, err := os.Open(path)
        if err != nil {
            return 0, err
        }

        entries, err := dir.Readdir(0)
        for _, entry := range entries {
            entryPath := filepath.Join(path, entry.Name())
            status, err := command.getStatus(entryPath, db)
            if err != nil {
                return 0, err
            }

            switch status {
            case TAGGED, MODIFIED, NESTED:
                return NESTED, err
            }
        }

        return UNTAGGED, nil
    }

    return UNTAGGED, nil
}

func isDir(path string) bool {
    info, err := os.Stat(path)
    if err != nil {
        return false
    }

    return info.IsDir()
}

type Status int

const (
    UNTAGGED Status = iota
    TAGGED
    MODIFIED
    NESTED
    MISSING
)