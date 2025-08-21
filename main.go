package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

type AutoRestart int

const (
	AUTORESTART_FALSE AutoRestart = iota
	AUTORESTART_TRUE
	AUTORESTART_UNEXPECTED
)

type Program struct {
	Cmd           string            `mapstructure:"cmd"`
	NumProcs      int               `mapstructure:"numprocs"`
	Umask         int               `mapstructure:"umask"`
	WorkingDir    string            `mapstructure:"workingdir"`
	User          string            `mapstructure:"user"`
	AutoStart     bool              `mapstructure:"autostart"`
	AutoRestart   AutoRestart       `mapstructure:"autorestart"`
	ExitCodes     []int             `mapstructure:"exitcodes"`
	StartRetries  int               `mapstructure:"startretries"`
	StartSecs     int               `mapstructure:"startsecs"`
	StopSignal    int               `mapstructure:"stopsignal"`
	StopSecs      int               `mapstructure:"stopsecs"`
	StdoutLogfile string            `mapstructure:"stdout_logfile"`
	StderrLogfile string            `mapstructure:"stderr_logfile"`
	Env           map[string]string `mapstructure:"env"`
}

func validateProgram(program Program) error {
	return errors.New("bla")
}

func defaultsHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		fmt.Println("NAME: ", f.Name())
		if t.Name() == "AutoRestart" {
			log.Fatal("wtf")
			switch v := data.(type) {
			case string:
				switch v {
				case "true":
					log.Fatal("wtf")
					return AUTORESTART_TRUE, nil
				case "false":
					log.Fatal("wtf")

					return AUTORESTART_FALSE, nil
				case "unexpected":
					log.Fatal("wtf")

					return AUTORESTART_UNEXPECTED, nil
				default:
					log.Fatal("wtf")

					return AUTORESTART_FALSE, nil // or return an error
				}
			default:
				log.Fatal("wtf")
			}
		}

		if t == reflect.TypeOf(Program{}) {
			program := Program{
				NumProcs:      1,
				AutoStart:     true,
				AutoRestart:   AUTORESTART_TRUE,
				ExitCodes:     []int{0},
				StartRetries:  3,
				StartSecs:     1,
				StopSignal:    15,
				StopSecs:      10,
				StdoutLogfile: "/tmp/stdout.log",
				StderrLogfile: "/tmp/stderr.log",
				Env:           make(map[string]string),
			}

			decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
				WeaklyTypedInput: true,
				Result:           &program,
			})
			decoder.Decode(data)
			return program, nil
		}
		return data, nil
	}
}

func main() {
	conf, err := os.ReadFile("taskmaster.conf")
	if err != nil {
		log.Fatal(err)
	}

	var raw any
	if err := yaml.Unmarshal(conf, &raw); err != nil {
		log.Fatal(err)
	}

	if rawPrograms, programsFound := raw.(map[string]any)["programs"]; programsFound {
		var programs map[string]Program

		decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result: &programs, WeaklyTypedInput: true, DecodeHook: defaultsHook()})
		if err := decoder.Decode(rawPrograms); err != nil {
			log.Fatal(err)
		}

		for _, program := range programs {
			fmt.Printf("%+v\n", program)
			err := validateProgram(program)
			if err != nil {

			}
		}
	}
}
