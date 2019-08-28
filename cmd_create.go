package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	cli "github.com/jawher/mow.cli"
)

func createProject(config *Config) func(*cli.Cmd) {
	return func(cmd *cli.Cmd) {
		cmd.Spec = "[--path | --parent] [TITLE]"
		cmd.LongDesc = `
Create a new project by specifying a title or path to a JSON file
containing a configuration. If - is specified as an argument to --path
configuration will be read from stdin.
        `
		var (
			title  = cmd.StringArg("TITLE", "", "project title")
			path   = cmd.StringOpt("path", "", "path to a project file")
			parent = cmd.IntOpt("p parent", 0, "parent project id")
		)
		cmd.Action = func() {
			project := &Project{}
			if *path != "" {
				if *path == "-" {
					maybe(json.NewDecoder(os.Stdin).Decode(project))
				} else {
					raw, err := ioutil.ReadFile(*path)
					maybe(err)
					maybe(json.Unmarshal(raw, project))
				}
			} else {
				if *title == "" {
					maybe(fmt.Errorf("need to specify a title or project file"))
				}
			}
			if *title != "" {
				project.Title = *title
			}
			if *parent != 0 {
				project.ParentID = int64(*parent)
			}
			store, err := NewSQLiteStore(config.DBPath)
			maybe(err)
			defer store.Close()
			maybe(store.With(func(s Store) error {
				return s.CreateProject(project)
			}))
		}
	}
}

func createTask(config *Config) func(*cli.Cmd) {
	return func(cmd *cli.Cmd) {
		cmd.Spec = "[OPTIONS] MESSAGE"
		var (
			projectId = cmd.IntOpt("project", 0, "project id")
			duration  = cmd.StringOpt("d duration", "25m", "duration of each stent")
			pomodoros = cmd.IntOpt("p pomodoros", 4, "number of pomodoros")
			message   = cmd.StringArg("MESSAGE", "", "descriptive name of the given task")
			tags      = cmd.StringsOpt("t tag", []string{}, "tags associated with this task")
		)
		cmd.Action = func() {
			parsed, err := time.ParseDuration(*duration)
			maybe(err)
			store, err := NewSQLiteStore(config.DBPath)
			maybe(err)
			defer store.Close()
			kvs, err := parseTags(*tags)
			maybe(err)
			task := &Task{
				ProjectID: int64(*projectId),
				Message:   *message,
				Duration:  parsed,
				Pomodoros: NewPomodoros(*pomodoros),
				Tags:      kvs,
			}
			maybe(store.With(func(s Store) error {
				return store.CreateTask(task)
			}))
			fmt.Printf("%d", task.ID)
		}
	}
}

func create(config *Config) func(*cli.Cmd) {
	return func(cmd *cli.Cmd) {
		cmd.Command("t task", "create a new task", createTask(config))
		cmd.Command("p project", "create a new project", createProject(config))
	}
}
