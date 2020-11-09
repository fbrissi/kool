package cmd

import (
	"strings"
	"errors"
	"fmt"
	"kool-dev/kool/cmd/presets"
	"kool-dev/kool/cmd/shell"

	"gopkg.in/yaml.v2"
	"github.com/spf13/cobra"
)

// KoolPresetFlags holds the flags for the preset command
type KoolPresetFlags struct {
	Override bool
}

// KoolPreset holds handlers and functions to implement the preset command logic
type KoolPreset struct {
	DefaultKoolService
	Flags        *KoolPresetFlags
	parser       presets.Parser
	terminal     shell.TerminalChecker
	promptSelect shell.PromptSelect
}

// ErrPresetFilesAlreadyExists error for existing presets files
var ErrPresetFilesAlreadyExists = errors.New("some preset files already exist")

func init() {
	var (
		preset    = NewKoolPreset()
		presetCmd = NewPresetCommand(preset)
	)

	rootCmd.AddCommand(presetCmd)
}

// NewKoolPreset creates a new handler for preset logic
func NewKoolPreset() *KoolPreset {
	return &KoolPreset{
		*newDefaultKoolService(),
		&KoolPresetFlags{false},
		&presets.DefaultParser{Presets: presets.GetAll()},
		shell.NewTerminalChecker(),
		shell.NewPromptSelect(),
	}
}

// Execute runs the preset logic with incoming arguments.
func (p *KoolPreset) Execute(args []string) (err error) {
	var (
		fileError, preset, language, database string
		defaultCompose bool
	)

	if len(args) == 0 {
		if !p.IsTerminal() {
			err = fmt.Errorf("the input device is not a TTY; for non-tty environments, please specify a preset argument")
			return
		}

		if language, err = p.promptSelect.Ask("What language do you want to use", p.parser.GetLanguages()); err != nil {
			return
		}

		if preset, err = p.promptSelect.Ask("What preset do you want to use", p.parser.GetPresets(language)); err != nil {
			return
		}

		if askDatabase := p.parser.GetPresetMetaValue(preset, "ask_database"); askDatabase != "" {
			dbOptions := strings.Split(askDatabase, ",")

			if database, err = p.promptSelect.Ask("What database do you want to use", dbOptions); err != nil {
				return
			}
		}

		defaultCompose = false
	} else {
		preset = args[0]
		defaultCompose = true
	}

	if !p.parser.Exists(preset) {
		err = fmt.Errorf("Unknown preset %s", preset)
		return
	}

	p.Println("Preset", preset, "is initializing!")

	if !p.Flags.Override {
		existingFiles := p.parser.LookUpFiles(preset)
		for _, fileName := range existingFiles {
			p.Warning("Preset file ", fileName, " already exists.")
		}

		if len(existingFiles) > 0 {
			err = ErrPresetFilesAlreadyExists
			return
		}
	}

	files := p.parser.GetPresetContents(preset)

	templates := presets.GetTemplates()

	for fileName, fileContent := range files {
		if fileName == "docker-compose.yml" && !defaultCompose {
			var dockerCompose, dockerComposeServices, appTempl, databaseTempl, cacheTempl yaml.MapSlice

			dockerCompose = append(dockerCompose, yaml.MapItem{Key: "version", Value: "3.7"})

			if appTempl, err = parseYml(templates["app"]["php74.yml"]); err != nil {
				err = fmt.Errorf("Failed to write preset file %s: %v", fileName, err)
				return
			}

			dockerComposeServices = append(dockerComposeServices, yaml.MapItem{Key: "app", Value: appTempl})

			if database != "" {
				databaseKey := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(database, " ", ""), ".", "")) + ".yml"

				if databaseTempl, err = parseYml(templates["database"][databaseKey]); err != nil {
					err = fmt.Errorf("Failed to write preset file %s: %v", fileName, err)
					return
				}

				dockerComposeServices = append(dockerComposeServices, yaml.MapItem{Key: "database", Value: databaseTempl})
			}

			if cacheTempl, err = parseYml(templates["cache"]["redis6.yml"]); err != nil {
				err = fmt.Errorf("Failed to write preset file %s: %v", fileName, err)
				return
			}

			dockerComposeServices = append(dockerComposeServices, yaml.MapItem{Key: "cache", Value: cacheTempl})

			dockerCompose = append(dockerCompose, yaml.MapItem{Key: "services", Value: dockerComposeServices})

			var parsedBytes []byte

			if parsedBytes, err = yaml.Marshal(dockerCompose); err != nil {
				err = fmt.Errorf("Failed to write preset file %s: %v", fileName, err)
				return
			}

			fileContent = string(parsedBytes)
		}

		if fileError, err = p.parser.WriteFile(fileName, fileContent); err != nil {
			err = fmt.Errorf("Failed to write preset file %s: %v", fileError, err)
			return
		}
	}

	p.Success("Preset ", preset, " initialized!")
	return
}

// NewPresetCommand initializes new kool preset command
func NewPresetCommand(preset *KoolPreset) (presetCmd *cobra.Command) {
	presetCmd = &cobra.Command{
		Use:   "preset [PRESET]",
		Short: "Initialize kool preset in the current working directory. If no preset argument is specified you will be prompted to pick among the existing options.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			preset.SetWriter(cmd.OutOrStdout())

			if err := preset.Execute(args); err != nil {
				if err.Error() == ErrPresetFilesAlreadyExists.Error() {
					preset.Warning("Some preset files already exist. In case you wanna override them, use --override.")
					preset.Exit(2)
				} else if err.Error() == shell.ErrPromptSelectInterrupted.Error() {
					preset.Warning("Operation Cancelled")
					preset.Exit(0)
				} else {
					preset.Error(err)
					preset.Exit(1)
				}
			}
		},
	}

	presetCmd.Flags().BoolVarP(&preset.Flags.Override, "override", "", false, "Force replace local existing files with the preset files")
	return
}

func parseYml(data string) (yaml.MapSlice, error) {
	parsed := yaml.MapSlice{}

	if err := yaml.Unmarshal([]byte(data), &parsed); err != nil {
		return nil, err
	}

	return parsed, nil
}
