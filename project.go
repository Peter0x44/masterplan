package main

import (
	"encoding/json"
	"fmt"
	"image/gif"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pkg/browser"
	"github.com/tanema/gween"
	"github.com/tanema/gween/ease"

	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"

	"github.com/gen2brain/dlgs"
	"github.com/gen2brain/raylib-go/raymath"

	"github.com/gabriel-vasile/mimetype"
	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	NUMBERING_SEQUENCE_NUMBER = iota
	NUMBERING_SEQUENCE_NUMBER_DASH
	NUMBERING_SEQUENCE_ROMAN
	NUMBERING_SEQUENCE_BULLET
	NUMBERING_SEQUENCE_OFF
)

const (

	// Task messages

	MessageNeighbors   = "neighbors"
	MessageNumbering   = "numbering"
	MessageDelete      = "delete"
	MessageSelect      = "select"
	MessageDropped     = "dropped"
	MessageDoubleClick = "double click"
	MessageDragging    = "dragging"
	MessageTaskClose   = "task close"

	// Project actions

	ActionNewProject    = "new"
	ActionLoadProject   = "load"
	ActionSaveAsProject = "save as"
)

var firstFreeTaskID = 0

type Project struct {
	// Settings / project-specific data
	FilePath string
	GridSize int32
	// Tasks                []Board
	Boards            []*Board
	BoardIndex        int
	BoardPanel        rl.Rectangle
	ZoomLevel         int
	CameraPan         rl.Vector2
	CameraOffset      rl.Vector2
	TaskShadowSpinner *Spinner
	GridVisible       *Checkbox
	// SampleRate           beep.SampleRate
	SampleRate           *Spinner
	SetSampleRate        int
	SampleBuffer         int
	ShowIcons            *Checkbox
	PulsingTaskSelection *Checkbox
	AutoSave             *Checkbox
	AutoReloadThemes     *Checkbox
	AutoLoadLastProject  *Checkbox
	DisableSplashscreen  *Checkbox
	SaveSoundsPlaying    *Checkbox
	OutlineTasks         *Checkbox
	ColorThemeSpinner    *Spinner

	// Internal data to make stuff work
	FullyInitialized        bool
	GridTexture             rl.Texture2D
	ContextMenuOpen         bool
	ContextMenuPosition     rl.Vector2
	ProjectSettingsOpen     bool
	RootPath                string
	Selecting               bool
	SelectionStart          rl.Vector2
	DoubleClickTimer        int
	DoubleClickTaskID       int
	CopyBuffer              []*Task
	Cutting                 bool // If cutting, then this boolean is set
	TaskOpen                bool
	ThemeReloadTimer        int
	NumberingSequence       *Spinner
	NumberingIgnoreTopLevel *Checkbox
	JustLoaded              bool
	ResizingImage           bool
	LogOn                   bool
	LoadRecentDropdown      *DropdownMenu

	SearchedTasks     []*Task
	FocusedSearchTask int
	Searchbar         *Textbox
	StatusBar         rl.Rectangle
	GUI_Icons         rl.Texture2D
	Patterns          rl.Texture2D
	ShortcutKeyTimer  int
	PreviousTaskType  string
	Resources         map[string]*Resource
	Modified          bool

	RenameBoardPopup   *TextboxPopup
	AbandonPlanPopup   *ButtonChoicePopup
	OverwritePlanPopup *ButtonChoicePopup
	ActivePopup        Popup
	PopupAction        string
	PopupArgument      string

	//UndoBuffer		// This is going to be difficult, because it needs to store a set of changes to execute for each change;
	// There's two ways to go about this I suppose. 1) Store the changes to disk whenever a change happens, then restore it when you undo, and vice-versa when redoing.
	// This would be simple, but could be prohibitive if the size becomes large. Upside is that since we're storing the buffer to disk, you can undo
	// things even between program sessions which is pretty insane.
	// 2) Make actual functions, I guess, for each user-controllable change that can happen to the project, and then store references to these functions
	// in a buffer; then walk backwards through them to change them, I suppose?
}

func NewProject() *Project {

	searchBar := NewTextbox(float32(rl.GetScreenWidth())-128, float32(float32(rl.GetScreenHeight()))-23, 128, 23)
	searchBar.MaxSize = searchBar.MinSize // Don't expand for text
	searchBar.AllowNewlines = false

	px := float32(390)

	project := &Project{
		FilePath:           "",
		GridSize:           16,
		ZoomLevel:          -99,
		CameraPan:          rl.Vector2{float32(rl.GetScreenWidth()) / 2, float32(rl.GetScreenHeight()) / 2},
		Searchbar:          searchBar,
		StatusBar:          rl.Rectangle{0, float32(rl.GetScreenHeight()) - 32, float32(rl.GetScreenWidth()), 32},
		GUI_Icons:          rl.LoadTexture(GetPath("assets", "gui_icons.png")),
		SampleBuffer:       512,
		Patterns:           rl.LoadTexture(GetPath("assets", "patterns.png")),
		Resources:          map[string]*Resource{},
		LoadRecentDropdown: NewDropdown(0, 0, 0, 0, "Load Recent..."), // Position and size is set below in the context menu handling

		ColorThemeSpinner:       NewSpinner(px, 0, 256, 24),
		TaskShadowSpinner:       NewSpinner(px, 0, 128, 24, "Off", "Flat", "Smooth", "3D"),
		OutlineTasks:            NewCheckbox(px, 0, 32, 32),
		GridVisible:             NewCheckbox(px, 0, 32, 32),
		ShowIcons:               NewCheckbox(px, 0, 32, 32),
		NumberingSequence:       NewSpinner(px, 0, 128, 24, "1.1.", "1-1)", "I.I.", "Bullets", "Off"),
		NumberingIgnoreTopLevel: NewCheckbox(px, 0, 32, 32),
		PulsingTaskSelection:    NewCheckbox(px, 0, 32, 32),
		AutoSave:                NewCheckbox(px, 0, 32, 32),
		AutoReloadThemes:        NewCheckbox(px, 0, 32, 32),
		SaveSoundsPlaying:       NewCheckbox(px, 0, 32, 32),
		SampleRate:              NewSpinner(px, 0, 128, 24, "22050", "44100", "48000", "88200", "96000"),
		DisableSplashscreen:     NewCheckbox(px, 0, 32, 32),
		AutoLoadLastProject:     NewCheckbox(px, 0, 32, 32),

		RenameBoardPopup:   NewTextboxPopup("New Board name:", "Accept", "Cancel"),
		AbandonPlanPopup:   NewButtonChoicePopup("This plan has been modified; Abandon plan?", "Yes", "No"),
		OverwritePlanPopup: NewButtonChoicePopup("A plan exists in this folder already. Overwrite?", "Yes", "No"),
	}

	// Position the settings using something more maintainable than adding 40 to each Y value in a line
	settingsOptions := []interface{}{
		project.ColorThemeSpinner,
		project.TaskShadowSpinner,
		project.OutlineTasks,
		project.GridVisible,
		project.ShowIcons,
		project.NumberingSequence,
		project.NumberingIgnoreTopLevel,
		project.PulsingTaskSelection,
		project.AutoSave,
		project.SaveSoundsPlaying,
		project.SampleRate,
		nil,
		project.AutoReloadThemes,
		project.AutoLoadLastProject,
		project.DisableSplashscreen,
	}

	y := float32(32)

	for _, option := range settingsOptions {

		if cb, isCB := option.(*Checkbox); isCB {
			cb.Rect.Y = y
		} else if sp, isSP := option.(*Spinner); isSP {
			sp.Rect.Y = y
		}

		y += 40
	}

	project.Boards = []*Board{NewBoard(project)}

	project.OutlineTasks.Checked = true
	project.LogOn = true
	project.PulsingTaskSelection.Checked = true
	project.TaskShadowSpinner.CurrentChoice = 2
	project.GridVisible.Checked = true
	project.ShowIcons.Checked = true
	project.DoubleClickTimer = -1
	project.PreviousTaskType = "Check Box"

	currentTheme = "Sunlight" // Default theme for new projects and new sessions is the Sunlight theme

	project.ReloadThemes()
	project.ChangeTheme(currentTheme)

	if strings.Contains(runtime.GOOS, "darwin") {
		project.SampleRate.SetChoice("22050") // For some reason, sound on Mac is choppy unless the project's sample rate is 22050.
	} else {
		project.SampleRate.SetChoice("44100")
	}
	speaker.Init(beep.SampleRate(project.SampleRate.ChoiceAsInt()), project.SampleBuffer)
	project.SetSampleRate = project.SampleRate.ChoiceAsInt()

	return project

}

func (project *Project) CurrentBoard() *Board {
	return project.Boards[project.BoardIndex]
}

func (project *Project) GetAllTasks() []*Task {
	tasks := []*Task{}
	for _, b := range project.Boards {
		tasks = append(tasks, b.Tasks...)
	}
	return tasks
}

func (project *Project) SaveAs() {
	dirPath, success, _ := dlgs.File("Select Project Directory", "", true)

	if success {

		projectFilepath := filepath.Join(dirPath, "master.plan")

		if FileExists(projectFilepath) {
			project.PopupAction = ActionSaveAsProject
			project.PopupArgument = projectFilepath
			project.ActivatePopup(project.OverwritePlanPopup)
		} else {
			project.ExecuteDestructiveAction(ActionSaveAsProject, projectFilepath)
		}

	}

}

func (project *Project) Save() {

	success := true

	if project.FilePath != "" {

		// Sort the Tasks by their ID, then loop through them using that slice. This way,
		// They store data according to their creation ID, not according to their position
		// in the world.
		tasksByID := append([]*Task{}, project.GetAllTasks()...)

		sort.Slice(tasksByID, func(i, j int) bool { return tasksByID[i].ID < tasksByID[j].ID })

		taskData := []map[string]interface{}{}
		for _, task := range tasksByID {
			if task.Serializable() {
				taskData = append(taskData, task.Serialize())
			}
		}

		data := map[string]interface{}{
			"Version":                 softwareVersion.String(),
			"GridSize":                project.GridSize,
			"Pan.X":                   project.CameraPan.X,
			"Pan.Y":                   project.CameraPan.Y,
			"ZoomLevel":               project.ZoomLevel,
			"BoardCount":              len(project.Boards),
			"Tasks":                   taskData,
			"ColorTheme":              currentTheme,
			"SampleRate":              project.SampleRate.ChoiceAsInt(),
			"SampleBuffer":            project.SampleBuffer,
			"TaskShadow":              project.TaskShadowSpinner.CurrentChoice,
			"OutlineTasks":            project.OutlineTasks.Checked,
			"GridVisible":             project.GridVisible.Checked,
			"ShowIcons":               project.ShowIcons.Checked,
			"NumberingIgnoreTopLevel": project.NumberingIgnoreTopLevel.Checked,
			"NumberingSequence":       project.NumberingSequence.CurrentChoice,
			"PulsingTaskSelection":    project.PulsingTaskSelection.Checked,
			"AutoSave":                project.AutoSave.Checked,
			"AutoReloadThemes":        project.AutoReloadThemes.Checked,
			"SaveSoundsPlaying":       project.SaveSoundsPlaying.Checked,
			"BoardIndex":              project.BoardIndex,
		}

		boardNames := []string{}
		for _, board := range project.Boards {
			boardNames = append(boardNames, board.Name)
		}

		data["BoardNames"] = boardNames

		f, err := os.Create(project.FilePath)
		if err != nil {
			log.Println(err)
			return
		} else {
			defer f.Close()
			encoder := json.NewEncoder(f)
			encoder.SetIndent("", "\t")
			encoder.Encode(data)
			programSettings.Save()

			err = f.Sync() // Want to make sure the file is written
			if err != nil {
				log.Println("ERROR: Can't write save file to system.", err)
				success = false
			}

		}

	} else {
		success = false
	}

	if success {
		project.Log("Save successful.")
		project.Modified = false
	} else {
		project.Log("ERROR: Save unsuccessful.")
	}

}

func (project *Project) LoadFrom() bool {
	// I used to have the extension for this file selector set to "*.plan", but Mac doesn't seem to recognize
	// MasterPlan's .plan files as having that extension... I'm just removing the extension filter for now.
	file, success, _ := dlgs.File("Load Plan File", "", false)
	if success {
		currentProject.Destroy()
		currentProject = NewProject()
		// TODO: DO something if this fails
		return currentProject.Load(file)
	}
	return false
}

func (project *Project) Load(filepath string) bool {

	success := true

	f, err := os.Open(filepath)
	if err != nil {
		log.Println(err)
		success = false
	} else {

		defer f.Close()
		decoder := json.NewDecoder(f)
		data := map[string]interface{}{}
		decoder.Decode(&data)

		dataGood := true

		if len(data) != 0 {
			_, exists := data["Tasks"]
			if !exists {
				dataGood = false
			}
		} else {
			dataGood = false
		}

		if dataGood {

			project.JustLoaded = true

			project.FilePath = filepath

			getFloat := func(name string, defaultValue float32) float32 {
				value, exists := data[name]
				if exists {
					return float32(value.(float64))
				} else {
					return defaultValue
				}
			}

			getInt := func(name string, defaultValue int) int {
				value, exists := data[name]
				if exists {
					return int(value.(float64))
				} else {
					return defaultValue
				}
			}

			getString := func(name string, defaultValue string) string {
				value, exists := data[name]
				if exists {
					return value.(string)
				} else {
					return defaultValue
				}
			}

			getStringArray := func(name string, defaultValue []string) []string {
				array, exists := data[name]
				if exists {
					data := []string{}
					for _, v := range array.([]interface{}) {
						data = append(data, v.(string))
					}
					return data
				} else {
					return defaultValue
				}
			}

			getBool := func(name string, defaultValue bool) bool {
				value, exists := data[name]
				if exists {
					return value.(bool)
				} else {
					return defaultValue
				}
			}

			project.GridSize = int32(getInt("GridSize", int(project.GridSize)))
			project.CameraPan.X = getFloat("Pan.X", project.CameraPan.X)
			project.CameraPan.Y = getFloat("Pan.Y", project.CameraPan.Y)
			project.ZoomLevel = getInt("ZoomLevel", project.ZoomLevel)
			project.SampleRate.SetChoice(string(getInt("SampleRate", project.SampleRate.ChoiceAsInt())))
			project.SampleBuffer = getInt("SampleBuffer", project.SampleBuffer)
			project.TaskShadowSpinner.CurrentChoice = getInt("TaskShadow", project.TaskShadowSpinner.CurrentChoice)
			project.OutlineTasks.Checked = getBool("OutlineTasks", project.OutlineTasks.Checked)
			project.GridVisible.Checked = getBool("GridVisible", project.GridVisible.Checked)
			project.ShowIcons.Checked = getBool("ShowIcons", project.ShowIcons.Checked)
			project.NumberingSequence.CurrentChoice = getInt("NumberingSequence", project.NumberingSequence.CurrentChoice)
			project.NumberingIgnoreTopLevel.Checked = getBool("NumberingIgnoreTopLevel", project.NumberingIgnoreTopLevel.Checked)
			project.PulsingTaskSelection.Checked = getBool("PulsingTaskSelection", project.PulsingTaskSelection.Checked)
			project.AutoSave.Checked = getBool("AutoSave", project.AutoSave.Checked)
			project.AutoReloadThemes.Checked = getBool("AutoReloadThemes", project.AutoReloadThemes.Checked)
			project.SaveSoundsPlaying.Checked = getBool("SaveSoundsPlaying", project.SaveSoundsPlaying.Checked)
			project.BoardIndex = getInt("BoardIndex", project.BoardIndex)

			speaker.Init(beep.SampleRate(project.SampleRate.ChoiceAsInt()), project.SampleBuffer)
			project.SetSampleRate = project.SampleRate.ChoiceAsInt()

			project.LogOn = false

			boardNames := getStringArray("BoardNames", []string{})

			for i := 0; i < getInt("BoardCount", 0)-1; i++ {
				project.AddBoard()
			}

			for i := range project.Boards {
				if i < len(boardNames) {
					project.Boards[i].Name = boardNames[i]
				}
			}

			for _, t := range data["Tasks"].([]interface{}) {
				taskData := t.(map[string]interface{})

				bi, exists := taskData["BoardIndex"]
				boardIndex := 0

				if exists {
					boardIndex = int(bi.(float64))
				}

				task := project.Boards[boardIndex].CreateNewTask()
				task.Deserialize(taskData)
			}

			project.LogOn = true

			colorTheme := getString("ColorTheme", currentTheme)
			if colorTheme != "" {
				project.ChangeTheme(colorTheme) // Changing theme regenerates the grid; we don't have to do it elsewhere
			}

			list := []string{}

			existsInList := func(value string) bool {
				for _, item := range list {
					if value == item {
						return true
					}
				}
				return false
			}

			lastOpenedIndex := -1
			i := 0
			for _, s := range programSettings.RecentPlanList {
				_, err := os.Stat(s)
				if err == nil && !existsInList(s) {
					// If err != nil, the file must not exist, so we'll skip it
					list = append(list, s)
					if s == filepath {
						lastOpenedIndex = i
					}
					i++
				}
			}

			if lastOpenedIndex > 0 {

				// If the project to be opened is already in the recent files list, then we can just bump it up to the front.

				// ABC <- Say we want to move B to the front.

				// list = ABC_
				list = append(list, "")

				// list = AABC
				copy(list[1:], list[0:])

				// list = BABC
				list[0] = list[lastOpenedIndex+1] // Index needs to be +1 here because we made the list 1 larger above

				// list = BAC
				list = append(list[:lastOpenedIndex+1], list[lastOpenedIndex+2:]...)

			} else if lastOpenedIndex < 0 {
				list = append([]string{filepath}, list...)
			}

			programSettings.RecentPlanList = list

			programSettings.Save()
			project.Log("Load successful.")

		} else {

			// It's possible for the file to be mangled and unable to be loaded; I should actually handle this
			// with a backup system or something.
			log.Println("Error: Could not load plan: [ %s ].", filepath)
			currentProject.Log("Error: Could not load plan: [ %s ].", filepath)
			currentProject.Log("Are you sure it's a valid MasterPlan project?")
			success = false

		}

	}

	return success

}

func (project *Project) Log(text string, variables ...interface{}) {
	if project.LogOn {
		if len(variables) > 0 {
			text = fmt.Sprintf(text, variables...)
		}
		eventLogBuffer = append(eventLogBuffer, EventLog{time.Now(), text, gween.New(255, 0, 7, ease.InCubic)})
	}
}

func (project *Project) HandleCamera() {

	wheel := rl.GetMouseWheelMove()

	if !project.ContextMenuOpen && !project.ProjectSettingsOpen && !project.TaskOpen {
		if wheel > 0 {
			project.ZoomLevel += 1
		} else if wheel < 0 {
			project.ZoomLevel -= 1
		}
	}

	zoomLevels := []float32{0.5, 1, 2, 3, 4}

	if project.ZoomLevel == -99 {
		project.ZoomLevel = 1
	}

	if project.ZoomLevel >= len(zoomLevels) {
		project.ZoomLevel = len(zoomLevels) - 1
	}

	if project.ZoomLevel < 0 {
		project.ZoomLevel = 0
	}

	targetZoom := zoomLevels[project.ZoomLevel]

	camera.Zoom += (targetZoom - camera.Zoom) * 0.2

	if math.Abs(float64(targetZoom-camera.Zoom)) < 0.001 {
		camera.Zoom = targetZoom
	}

	if rl.IsMouseButtonDown(rl.MouseMiddleButton) {
		diff := GetMouseDelta()
		project.CameraPan.X += diff.X
		project.CameraPan.Y += diff.Y
	}

	project.CameraOffset.X += float32(project.CameraPan.X-project.CameraOffset.X) * 0.2
	project.CameraOffset.Y += float32(project.CameraPan.Y-project.CameraOffset.Y) * 0.2

	camera.Target.X = float32(-project.CameraOffset.X)
	camera.Target.Y = float32(-project.CameraOffset.Y)

	camera.Offset.X = float32(rl.GetScreenWidth() / 2)
	camera.Offset.Y = float32(rl.GetScreenHeight() / 2)

}

func (project *Project) MousingOver() string {

	if rl.CheckCollisionPointRec(GetMousePosition(), project.StatusBar) {
		return "StatusBar"
	} else if rl.CheckCollisionPointRec(GetMousePosition(), project.BoardPanel) {
		return "Boards"
	} else if project.TaskOpen {
		return "TaskOpen"
	} else {
		return "Project"
	}

}

func (project *Project) Update() {

	if project.ActivePopup == nil {

		if project.AutoReloadThemes.Checked && project.ThemeReloadTimer > 30 {
			project.ReloadThemes()
			project.ThemeReloadTimer = 0
		}
		project.ThemeReloadTimer++

		holdingShift := rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
		holdingAlt := rl.IsKeyDown(rl.KeyLeftAlt) || rl.IsKeyDown(rl.KeyRightAlt)

		src := rl.Rectangle{-100000, -100000, 200000, 200000}
		dst := src
		rl.DrawTexturePro(project.GridTexture, src, dst, rl.Vector2{}, 0, rl.White)

		// Board name on background of project
		boardName := project.CurrentBoard().Name
		boardNameWidth := GUITextWidth(boardName) + 16
		boardNameHeight, _ := TextHeight(boardName, true)
		rl.DrawRectangle(1, 1, int32(boardNameWidth), int32(boardNameHeight), getThemeColor(GUI_INSIDE))
		DrawGUITextColored(rl.Vector2{8, 0}, getThemeColor(GUI_INSIDE_DISABLED), boardName)

		// This is the origin crosshair
		rl.DrawLineEx(rl.Vector2{0, -100000}, rl.Vector2{0, 100000}, 2, getThemeColor(GUI_INSIDE))
		rl.DrawLineEx(rl.Vector2{-100000, 0}, rl.Vector2{100000, 0}, 2, getThemeColor(GUI_INSIDE))

		selectionRect := rl.Rectangle{}

		if !project.TaskOpen && !project.ProjectSettingsOpen {

			project.CurrentBoard().HandleDroppedFiles()
			project.HandleCamera()

			var clickedTask *Task
			clicked := false

			// We update the tasks from top (last) down, because if you click on one, you click on the top-most one.

			if rl.IsMouseButtonPressed(rl.MouseLeftButton) && !project.ContextMenuOpen && !project.ProjectSettingsOpen {
				clicked = true
			}

			if project.ResizingImage {
				project.Selecting = false
			}

			if project.MousingOver() == "Project" {

				for i := len(project.CurrentBoard().Tasks) - 1; i >= 0; i-- {

					task := project.CurrentBoard().Tasks[i]

					if rl.CheckCollisionPointRec(GetWorldMousePosition(), task.Rect) && clickedTask == nil {
						clickedTask = task
					}

				}

				if project.DoubleClickTimer >= 0 {
					project.DoubleClickTimer++
				}

				if project.DoubleClickTimer >= 20 {
					project.DoubleClickTimer = -1
				}

				if clicked {

					if clickedTask == nil {
						project.SelectionStart = GetWorldMousePosition()
						project.Selecting = true
					} else {
						project.Selecting = false

						if holdingAlt && clickedTask.Selected {
							project.Log("Deselected 1 Task.")
						} else if !holdingAlt && !clickedTask.Selected {
							project.Log("Selected 1 Task.")
						}

						if holdingShift {

							if holdingAlt {
								clickedTask.ReceiveMessage(MessageSelect, map[string]interface{}{})
							} else {
								clickedTask.ReceiveMessage(MessageSelect, map[string]interface{}{
									"task": clickedTask,
								})
							}

						} else {
							if !clickedTask.Selected { // This makes it so you don't have to shift+drag to move already selected Tasks
								project.SendMessage(MessageSelect, map[string]interface{}{
									"task": clickedTask,
								})
							} else {
								clickedTask.ReceiveMessage(MessageSelect, map[string]interface{}{
									"task": clickedTask,
								})
							}
						}

					}

					if clickedTask == nil {

						if project.DoubleClickTimer > 0 && project.DoubleClickTaskID == -1 {
							task := project.CurrentBoard().CreateNewTask()
							task.ReceiveMessage(MessageDoubleClick, nil)
							project.Selecting = false
						}

						project.DoubleClickTaskID = -1
						project.DoubleClickTimer = 0

					} else {

						if clickedTask.ID == project.DoubleClickTaskID && project.DoubleClickTimer > 0 && clickedTask.Selected {
							clickedTask.ReceiveMessage(MessageDoubleClick, nil)
						} else {
							project.SendMessage(MessageDragging, nil)
						}

						project.DoubleClickTimer = 0
						project.DoubleClickTaskID = clickedTask.ID
					}

				}

				if project.Selecting {

					diff := raymath.Vector2Subtract(GetWorldMousePosition(), project.SelectionStart)
					x1, y1 := project.SelectionStart.X, project.SelectionStart.Y
					x2, y2 := diff.X, diff.Y
					if x2 < 0 {
						x2 *= -1
						x1 = GetWorldMousePosition().X
					}
					if y2 < 0 {
						y2 *= -1
						y1 = GetWorldMousePosition().Y
					}

					selectionRect = rl.Rectangle{x1, y1, x2, y2}

					if rl.IsMouseButtonReleased(rl.MouseLeftButton) && !project.ResizingImage {

						project.Selecting = false // We're done with the selection process

						count := 0

						for _, task := range project.CurrentBoard().Tasks {

							inSelectionRect := false
							var t *Task

							if rl.CheckCollisionRecs(selectionRect, task.Rect) {
								inSelectionRect = true
								t = task
							}

							if holdingAlt {
								if inSelectionRect {

									if task.Selected {
										count++
									}

									task.ReceiveMessage(MessageSelect, map[string]interface{}{"task": t, "invert": true})

								}
							} else {

								if !holdingShift || inSelectionRect {

									if (!task.Selected && inSelectionRect) || (!holdingShift && inSelectionRect) {
										count++
									}

									task.ReceiveMessage(MessageSelect, map[string]interface{}{
										"task": t,
									})

								}

							}

						}

						if holdingAlt {
							project.Log("Deselected %d Task(s).", count)
						} else {
							project.Log("Selected %d Task(s).", count)
						}

					}

				}

			} else {
				if rl.IsMouseButtonReleased(rl.MouseLeftButton) {
					project.Selecting = false
				}
			}

		}

		for _, task := range project.GetAllTasks() {
			task.Update()
		}

		// Additive blending should be out here to avoid state changes mid-task drawing.
		shadowColor := getThemeColor(GUI_SHADOW_COLOR)

		if shadowColor.R > 254 || shadowColor.G > 254 || shadowColor.B > 254 {
			rl.BeginBlendMode(rl.BlendAdditive)
		}

		for _, task := range project.CurrentBoard().Tasks {
			task.DrawShadow()
		}

		if shadowColor.R > 254 || shadowColor.G > 254 || shadowColor.B > 254 {
			rl.EndBlendMode()
		}

		for _, task := range project.CurrentBoard().Tasks {
			task.Draw()
		}

		// This is true once at least one loop has happened
		project.FullyInitialized = true

		rl.DrawRectangleLinesEx(selectionRect, 1, getThemeColor(GUI_OUTLINE_HIGHLIGHTED))

		project.Shortcuts()

		if project.JustLoaded {

			for _, t := range project.GetAllTasks() {
				t.Draw() // We need to draw the task at least once to ensure the rects are updated by the Task's contents.
				// This makes it so that neighbors can be correct.
			}

			project.ReorderTasks()
			project.Modified = false
			project.JustLoaded = false
		}

	}

	for _, board := range project.Boards {
		board.HandleDeletedTasks()
	}

}

func (project *Project) SendMessage(message string, data map[string]interface{}) {

	taskList := project.GetAllTasks()

	for _, task := range taskList {
		task.ReceiveMessage(message, data)
	}

	if project.AutoSave.Checked {
		project.Save() // Save whenever anything important happens
	} else {
		if message == MessageDelete || message == MessageDragging || message == MessageTaskClose || message == MessageDropped || message == MessageSelect {
			project.Modified = true
		}
	}

}

func (project *Project) Shortcuts() {

	repeatKeys := []int32{
		rl.KeyUp,
		rl.KeyDown,
		rl.KeyLeft,
		rl.KeyRight,
		rl.KeyF,
		rl.KeyEnter,
		rl.KeyKpEnter,
	}

	repeatableKeyDown := map[int32]bool{}

	for _, key := range repeatKeys {
		repeatableKeyDown[key] = false

		if rl.IsKeyPressed(key) {
			project.ShortcutKeyTimer = 0
			repeatableKeyDown[key] = true
		} else if rl.IsKeyDown(key) {
			project.ShortcutKeyTimer++
			if project.ShortcutKeyTimer >= 30 && project.ShortcutKeyTimer%2 == 0 {
				repeatableKeyDown[key] = true
			}
		} else if rl.IsKeyReleased(key) {
			project.ShortcutKeyTimer = 0
		}
	}

	holdingShift := rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
	holdingCtrl := rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)

	if strings.Contains(runtime.GOOS, "darwin") && !holdingCtrl {
		holdingCtrl = rl.IsKeyDown(rl.KeyLeftSuper) || rl.IsKeyDown(rl.KeyRightSuper)
	}

	if !project.ProjectSettingsOpen {

		if !project.TaskOpen {

			if !project.Searchbar.Focused {

				panSpeed := float32(16 / camera.Zoom)
				selectedTasks := project.CurrentBoard().SelectedTasks(false)
				gs := float32(project.GridSize)

				if holdingShift {
					panSpeed *= 3
				}

				if !holdingCtrl && rl.IsKeyDown(rl.KeyW) {
					project.CameraPan.Y += panSpeed
				}
				if !holdingCtrl && rl.IsKeyDown(rl.KeyS) {
					project.CameraPan.Y -= panSpeed
				}
				if !holdingCtrl && rl.IsKeyDown(rl.KeyA) {
					project.CameraPan.X += panSpeed
				}
				if !holdingCtrl && rl.IsKeyDown(rl.KeyD) {
					project.CameraPan.X -= panSpeed
				}

				if holdingCtrl && rl.IsKeyPressed(rl.KeyOne) {
					if len(project.Boards) > 0 {
						project.BoardIndex = 0
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyTwo) {
					if len(project.Boards) > 1 {
						project.BoardIndex = 1
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyThree) {
					if len(project.Boards) > 2 {
						project.BoardIndex = 2
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyFour) {
					if len(project.Boards) > 3 {
						project.BoardIndex = 3
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyFive) {
					if len(project.Boards) > 4 {
						project.BoardIndex = 4
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeySix) {
					if len(project.Boards) > 5 {
						project.BoardIndex = 5
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeySeven) {
					if len(project.Boards) > 6 {
						project.BoardIndex = 6
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyEight) {
					if len(project.Boards) > 7 {
						project.BoardIndex = 7
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyNine) {
					if len(project.Boards) > 8 {
						project.BoardIndex = 8
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyZero) {
					if len(project.Boards) > 9 {
						project.BoardIndex = 9
					}
				} else if rl.IsKeyPressed(rl.KeyOne) || rl.IsKeyPressed(rl.KeyKp1) {
					project.ZoomLevel = 0
				} else if rl.IsKeyPressed(rl.KeyTwo) || rl.IsKeyPressed(rl.KeyKp2) {
					project.ZoomLevel = 1
				} else if rl.IsKeyPressed(rl.KeyThree) || rl.IsKeyPressed(rl.KeyKp3) {
					project.ZoomLevel = 2
				} else if rl.IsKeyPressed(rl.KeyFour) || rl.IsKeyPressed(rl.KeyKp4) {
					project.ZoomLevel = 3
				} else if rl.IsKeyPressed(rl.KeyFive) || rl.IsKeyPressed(rl.KeyKp5) {
					project.ZoomLevel = 4
				} else if rl.IsKeyPressed(rl.KeyBackspace) {
					project.CameraPan.X = 0
					project.CameraPan.Y = 0
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyA) {

					for _, task := range project.CurrentBoard().Tasks {
						task.Selected = true
					}

					project.Log("Selected all %d Task(s).", len(project.CurrentBoard().Tasks))

				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyC) {
					project.CurrentBoard().CopySelectedTasks()
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyX) {
					project.CurrentBoard().CutSelectedTasks()
				} else if holdingCtrl && holdingShift && rl.IsKeyPressed(rl.KeyV) {
					project.CurrentBoard().PasteContent()
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyV) {
					project.CurrentBoard().PasteTasks()
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyN) {
					task := project.CurrentBoard().CreateNewTask()
					task.ReceiveMessage(MessageDoubleClick, nil)
				} else if holdingShift && rl.IsKeyPressed(rl.KeyC) {

					for _, task := range project.GetAllTasks() {
						task.StopSound()
					}
					project.Log("Stopped all playing Sounds.")

				} else if rl.IsKeyPressed(rl.KeyC) {

					toggleCount := 0

					for _, task := range project.CurrentBoard().SelectedTasks(false) {
						if task.Completable() {
							toggleCount++
						}
						task.SetCompletion(!task.IsComplete())
					}

					if toggleCount > 0 {
						project.Log("Completion toggled on %d Task(s).", toggleCount)
					}

				} else if rl.IsKeyPressed(rl.KeyDelete) {
					project.CurrentBoard().DeleteSelectedTasks()
				} else if rl.IsKeyPressed(rl.KeyF) {
					project.CurrentBoard().FocusViewOnSelectedTasks()
				} else if len(selectedTasks) > 0 && (repeatableKeyDown[rl.KeyUp] ||
					repeatableKeyDown[rl.KeyRight] ||
					repeatableKeyDown[rl.KeyDown] ||
					repeatableKeyDown[rl.KeyLeft]) {

					// Selecting + sliding

					up := repeatableKeyDown[rl.KeyUp]
					right := repeatableKeyDown[rl.KeyRight]
					down := repeatableKeyDown[rl.KeyDown]
					left := repeatableKeyDown[rl.KeyLeft]

					if holdingCtrl {

						// Shift Tasks / Slide Tasks

						move := []float32{0, 0}

						if up {
							move[1] = -gs
						} else if down {
							move[1] = gs
						}

						if right {
							move[0] = gs
						} else if left {
							move[0] = -gs
						}

						neighborList := []*Task{}

						for _, task := range selectedTasks {

							// Arrows that point to Tasks
							// arrowNeighbors := []*Task{
							// 	task.TaskAbove,
							// 	task.TaskRight,
							// 	task.TaskLeft,
							// 	task.TaskBelow,
							// }

							// for _, arrow := range arrowNeighbors {
							// 	if arrow != nil && arrow.ArrowPointingToTask == task {
							// 		arrow.Position.X += move[0]
							// 		arrow.Position.Y += move[1]
							// 	}
							// }

							// Not quite working because arrows won't move if they're attached to a neighbor that you're sliding around

							if neighbor := task.NeighborInDirection(move[0], move[1]); task.Numberable() &&
								neighbor != nil && neighbor.Numberable() {

								if !neighbor.Selected {
									neighborList = append(neighborList, neighbor)
								}

							}

							// if task.Numberable() && neighbor != nil && neighbor.Numberable() {

							// 	if !neighbor.Selected {
							// 		neighborList = append(neighborList, neighbor)
							// 	}

							// 	task.Position.X += move[0]
							// 	task.Position.Y += move[1]

							// } else {
							// 	task.Move(move[0], move[1])
							// }

							task.Position.X += move[0]
							task.Position.Y += move[1]

						}

						project.ReorderTasks()

						for _, neighbor := range neighborList {
							neighbor.Move(-move[0], -move[1])
						}

						project.CurrentBoard().FocusViewOnSelectedTasks()
						project.ReorderTasks()

					} else {

						var selected *Task
						if down || right || left {
							selected = selectedTasks[len(selectedTasks)-1]
						} else {
							selected = selectedTasks[0]
						}

						if selected != nil {

							others := []*Task{}

							// Selection by keypress prioritizes neighbors first and foremost

							if right && selected.TaskRight != nil {

								others = []*Task{selected.TaskRight}

							} else if left && selected.TaskLeft != nil {

								others = []*Task{selected.TaskLeft}

							} else if up && selected.TaskAbove != nil {

								others = []*Task{selected.TaskAbove}

							} else if down && selected.TaskBelow != nil {

								others = []*Task{selected.TaskBelow}

							} else {

								for _, t := range selected.Board.Tasks {
									if right && t.Position.X > selected.Position.X {
										others = append(others, t)
									} else if down && t.Position.Y > selected.Position.Y {
										others = append(others, t)
									} else if left && t.Position.X < selected.Position.X {
										others = append(others, t)
									} else if up && t.Position.Y < selected.Position.Y {
										others = append(others, t)
									}
								}

								sort.Slice(others, func(i, j int) bool {
									return raymath.Vector2Distance(others[i].Position, selected.Position) <
										raymath.Vector2Distance(others[j].Position, selected.Position)
								})

							}

							var neighbor *Task
							if len(others) > 0 {
								neighbor = others[0]
							}

							if neighbor != nil {

								if holdingShift {
									neighbor.ReceiveMessage(MessageSelect, map[string]interface{}{"task": neighbor})
								} else {
									project.SendMessage(MessageSelect, map[string]interface{}{"task": neighbor})
								}

							}

							project.CurrentBoard().FocusViewOnSelectedTasks()

						}

					}

				} else if rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeyKpEnter) {
					for _, task := range project.CurrentBoard().SelectedTasks(true) {
						task.ReceiveMessage(MessageDoubleClick, nil)
					}
				} else if holdingCtrl && holdingShift && rl.IsKeyPressed(rl.KeyS) {

					// Project Shortcuts

					project.SaveAs()
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyS) {
					if project.FilePath == "" {
						project.SaveAs()
					} else {
						project.Save()
					}
				} else if holdingCtrl && rl.IsKeyPressed(rl.KeyO) {
					if project.Modified {
						project.PopupAction = ActionLoadProject
						project.ActivatePopup(project.AbandonPlanPopup)
					} else {
						project.ExecuteDestructiveAction(ActionLoadProject, "")
					}
				} else if rl.IsKeyPressed(rl.KeyEscape) {
					project.SendMessage(MessageSelect, nil)
					project.Log("Deselected all Task(s).")
				} else if rl.IsKeyPressed(rl.KeyPageUp) {
					for _, task := range project.CurrentBoard().SelectedTasks(true) {
						next := task.TaskAbove
						for next != nil && next.TaskAbove != nil {
							next = next.TaskAbove
						}
						if next != nil {
							project.SendMessage(MessageSelect, map[string]interface{}{"task": next})
						}
						break
					}
					project.CurrentBoard().FocusViewOnSelectedTasks()
				} else if rl.IsKeyPressed(rl.KeyPageDown) {
					for _, task := range project.CurrentBoard().Tasks {
						if task.Selected {
							next := task.TaskBelow
							for next != nil && next.TaskBelow != nil {
								next = next.TaskBelow
							}
							if next != nil {
								project.SendMessage(MessageSelect, map[string]interface{}{"task": next})
							}
							break
						}
					}
					project.CurrentBoard().FocusViewOnSelectedTasks()
				}

			}

			if project.Searchbar.Focused && (repeatableKeyDown[rl.KeyEnter] || repeatableKeyDown[rl.KeyKpEnter]) {
				if holdingShift {
					project.FocusedSearchTask--
				} else {
					project.FocusedSearchTask++
				}
				project.SearchForTasks()
			}

			if holdingCtrl && repeatableKeyDown[rl.KeyF] {
				if project.Searchbar.Focused {
					if holdingShift {
						project.FocusedSearchTask--
					} else {
						project.FocusedSearchTask++
					}
					project.SearchForTasks()
				} else {
					project.SearchForTasks()
					project.Searchbar.Focused = true
				}
			}

		} else if rl.IsKeyPressed(rl.KeyEscape) {
			project.SendMessage(MessageTaskClose, nil)
		}

	}

}

func (project *Project) ReorderTasks() {

	for _, board := range project.Boards {
		board.ReorderTasks()
	}

	project.SendMessage(MessageDropped, nil)
	project.SendMessage(MessageNeighbors, nil)
	project.SendMessage(MessageNumbering, nil)

}

func (project *Project) ChangeTheme(themeName string) {
	_, themeExists := guiColors[themeName]
	if themeExists {
		project.ColorThemeSpinner.SetChoice(themeName)
	} else {
		project.ColorThemeSpinner.CurrentChoice = 0 // Backup in case the named theme doesn't exist
	}
	currentTheme = project.ColorThemeSpinner.ChoiceAsString()
	project.GenerateGrid()
}

func (project *Project) GUI() {

	for _, task := range project.CurrentBoard().Tasks {
		task.PostDraw()
	}

	if project.ActivePopup != nil {

		project.ActivePopup.Update()

		result := project.ActivePopup.SelectedIndex()
		if rl.IsKeyPressed(rl.KeyEnter) || rl.IsKeyPressed(rl.KeyKpEnter) {
			result = 0
		} else if rl.IsKeyPressed(rl.KeyEscape) {
			result = len(project.ActivePopup.SelectionChoices()) - 1
		}

		if result >= 0 {

			if project.ActivePopup == project.RenameBoardPopup {

				if result == 0 {
					// Accept
					project.CurrentBoard().Name = project.RenameBoardPopup.Textbox.Text()
					project.Log("Renamed Board: %s", project.CurrentBoard().Name)
				}

			} else {
				if result == 0 {
					project.ExecuteDestructiveAction(project.PopupAction, project.PopupArgument)
				}
			}

			project.ActivePopup.Close()
			project.ActivePopup = nil

		}

	} else {

		if rl.IsMouseButtonReleased(rl.MouseRightButton) && !project.TaskOpen && !project.ContextMenuOpen {
			project.ContextMenuOpen = true
			project.ContextMenuPosition = GetMousePosition()
		} else if project.ContextMenuOpen {

			closeMenu := false

			pos := project.ContextMenuPosition

			menuOptions := []string{
				"New Project",
				"Load Project",
				"Load Recent...",
				"Save Project",
				"Save Project As...",
				"Project Settings",
				"",
				"New Task",
				"Delete Tasks",
				"Cut Tasks",
				"Copy Tasks",
				"Paste Tasks",
				"Paste Content",
				"",
				"Visit Forums",
				"Take Screenshot",
			}

			menuWidth := float32(192)
			menuHeight := float32(32 * len(menuOptions))

			pos.X -= menuWidth / 2
			pos.Y += 16

			if pos.X < 0 {
				pos.X = 0
			} else if pos.X > float32(rl.GetScreenWidth())-menuWidth {
				pos.X = float32(rl.GetScreenWidth()) - menuWidth
			}

			if pos.Y < menuHeight/2 {
				pos.Y = menuHeight / 2
			} else if pos.Y > float32(rl.GetScreenHeight())-menuHeight/2 {
				pos.Y = float32(rl.GetScreenHeight()) - menuHeight/2
			}

			rect := rl.Rectangle{pos.X, pos.Y, menuWidth, 32}

			newTaskPos := float32(1)
			for _, option := range menuOptions {
				if option == "New Task" {
					break
				} else if option == "" {
					newTaskPos += 0.5
				} else {
					newTaskPos++
				}
			}

			rect.Y -= (float32(newTaskPos) * rect.Height) // This to make it start on New Task by default

			selectedCount := len(project.CurrentBoard().SelectedTasks(false))

			for _, option := range menuOptions {

				disabled := option == "" // Spacer can't be selected

				if option == "Copy Tasks" && selectedCount == 0 ||
					option == "Delete Tasks" && selectedCount == 0 ||
					option == "Paste Tasks" && len(project.CopyBuffer) == 0 {
					disabled = true
				}

				if option == "Save Project" && project.FilePath == "" {
					disabled = true
				}

				if option == "" {
					rect.Height /= 2
				}

				if option == "Load Recent..." {

					project.LoadRecentDropdown.Rect = rect
					project.LoadRecentDropdown.Update()
					project.LoadRecentDropdown.Options = programSettings.RecentPlanList

					if len(programSettings.RecentPlanList) == 0 {
						project.LoadRecentDropdown.Options = []string{"No recent plans loaded"}
					} else if project.LoadRecentDropdown.ChoiceAsString() != "" {
						if project.Modified {
							project.PopupAction = ActionLoadProject
							project.PopupArgument = project.LoadRecentDropdown.ChoiceAsString()
							project.ActivatePopup(project.AbandonPlanPopup)
						} else {
							project.ExecuteDestructiveAction(ActionLoadProject, project.LoadRecentDropdown.ChoiceAsString())
						}
						closeMenu = true
					}

				} else if ImmediateButton(rect, option, disabled) {

					closeMenu = true

					switch option {

					case "New Project":
						if project.Modified {
							project.PopupAction = ActionNewProject
							project.ActivatePopup(project.AbandonPlanPopup)
						} else {
							project.ExecuteDestructiveAction(ActionNewProject, "")
						}

					case "Save Project":
						project.Save()

					case "Save Project As...":
						project.SaveAs()

					case "Load Project":
						if project.Modified {
							project.PopupAction = ActionLoadProject
							project.ActivatePopup(project.AbandonPlanPopup)
						} else {
							project.ExecuteDestructiveAction(ActionLoadProject, "")
						}

					case "Project Settings":
						project.ReloadThemes() // Reload the themes after opening the settings window
						project.ProjectSettingsOpen = true
						project.AutoLoadLastProject.Checked = programSettings.AutoloadLastPlan
						project.DisableSplashscreen.Checked = programSettings.DisableSplashscreen

					case "New Task":
						task := project.CurrentBoard().CreateNewTask()
						task.ReceiveMessage(MessageDoubleClick, nil)

					case "Delete Tasks":
						project.CurrentBoard().DeleteSelectedTasks()

					case "Cut Tasks":
						project.CurrentBoard().CutSelectedTasks()

					case "Copy Tasks":
						project.CurrentBoard().CopySelectedTasks()

					case "Paste Tasks":
						project.CurrentBoard().PasteTasks()

					case "Paste Content":
						project.CurrentBoard().PasteContent()

					case "Visit Forums":
						browser.OpenURL("https://solarlune.itch.io/masterplan/community")

					case "Take Screenshot":
						takeScreenshot = true

					}

				}

				rect.Y += rect.Height

				if option == "" {
					rect.Height *= 2
				}

			}

			if (rl.IsMouseButtonReleased(rl.MouseLeftButton) && !closeMenu && !project.LoadRecentDropdown.Clicked) || rl.IsMouseButtonReleased(rl.MouseMiddleButton) || rl.IsMouseButtonReleased(rl.MouseRightButton) {
				closeMenu = true
			}

			if closeMenu {
				project.ContextMenuOpen = false
				project.LoadRecentDropdown.Open = false
			}

		} else if project.ProjectSettingsOpen {

			rec := rl.Rectangle{16, 16, 750, project.DisableSplashscreen.Rect.Y + 32}
			rl.DrawRectangleRec(rec, getThemeColor(GUI_INSIDE))
			rl.DrawRectangleLinesEx(rec, 1, getThemeColor(GUI_OUTLINE))

			if ImmediateButton(rl.Rectangle{rec.Width - 16, rec.Y, 32, 32}, "X", false) {
				project.ProjectSettingsOpen = false

				if project.SampleRate.ChoiceAsInt() != project.SetSampleRate {
					speaker.Init(beep.SampleRate(project.SampleRate.ChoiceAsInt()), project.SampleBuffer)
					project.SetSampleRate = project.SampleRate.ChoiceAsInt()
					project.Log("Project sample rate changed to %s.", project.SampleRate.ChoiceAsString())
					project.Log("Currently playing sounds have been stopped and resampled as necessary.")
					project.LogOn = false
					for _, t := range project.CurrentBoard().Tasks {
						if t.TaskType.CurrentChoice == TASK_TYPE_SOUND {
							t.LoadResource(true) // Force reloading to resample as necessary
						}
					}
					project.LogOn = true
				}

				programSettings.AutoloadLastPlan = project.AutoLoadLastProject.Checked
				programSettings.DisableSplashscreen = project.DisableSplashscreen.Checked

				if project.AutoSave.Checked {
					project.Save()
				} else {
					// After modifying the project settings, the project probably has been modified
					project.Modified = true
				}
				programSettings.Save()
			}

			columnX := float32(32)

			DrawGUIText(rl.Vector2{columnX, project.TaskShadowSpinner.Rect.Y + 4}, "Task Depth: ")
			project.TaskShadowSpinner.Update()

			DrawGUIText(rl.Vector2{columnX, project.OutlineTasks.Rect.Y + 4}, "Outline Tasks: ")
			project.OutlineTasks.Update()

			DrawGUIText(rl.Vector2{columnX, project.ColorThemeSpinner.Rect.Y + 4}, "Color Theme: ")
			project.ColorThemeSpinner.Update()

			DrawGUIText(rl.Vector2{columnX, project.GridVisible.Rect.Y + 4}, "Grid Visible: ")
			project.GridVisible.Update()

			DrawGUIText(rl.Vector2{columnX, project.ShowIcons.Rect.Y + 4}, "Show Icons: ")
			project.ShowIcons.Update()

			if project.GridVisible.Changed {
				project.GenerateGrid()
			}

			if project.ColorThemeSpinner.Changed {
				project.ChangeTheme(project.ColorThemeSpinner.ChoiceAsString())
			}

			DrawGUIText(rl.Vector2{columnX, project.NumberingSequence.Rect.Y + 4}, "Numbering Sequence: ")
			project.NumberingSequence.Update()

			DrawGUIText(rl.Vector2{columnX, project.NumberingIgnoreTopLevel.Rect.Y + 4}, "Ignore Numbering Top-level Tasks:")
			project.NumberingIgnoreTopLevel.Update()

			DrawGUIText(rl.Vector2{columnX, project.PulsingTaskSelection.Rect.Y + 4}, "Pulsing Task Selection Outlines:")
			project.PulsingTaskSelection.Update()

			DrawGUIText(rl.Vector2{columnX, project.AutoSave.Rect.Y + 4}, "Auto-save Projects on Change:")
			project.AutoSave.Update()

			DrawGUIText(rl.Vector2{columnX, project.AutoReloadThemes.Rect.Y + 4}, "Auto-reload Themes:")
			project.AutoReloadThemes.Update()

			DrawGUIText(rl.Vector2{columnX, project.SampleRate.Rect.Y + 4}, "Project Samplerate:")
			project.SampleRate.Update()

			DrawGUIText(rl.Vector2{columnX, project.AutoLoadLastProject.Rect.Y + 4}, "Auto-load Last Saved Project:")
			project.AutoLoadLastProject.Update()

			DrawGUIText(rl.Vector2{columnX, project.SaveSoundsPlaying.Rect.Y + 4}, "Save Sound Playback Status:")
			project.SaveSoundsPlaying.Update()

			DrawGUIText(rl.Vector2{columnX, project.DisableSplashscreen.Rect.Y + 4}, "Disable Splashscreen on Start:")
			project.DisableSplashscreen.Update()

		}

		if !project.ProjectSettingsOpen {

			// Status bar

			project.StatusBar.Y = float32(rl.GetScreenHeight()) - project.StatusBar.Height
			project.StatusBar.Width = float32(rl.GetScreenWidth())

			rl.DrawRectangleRec(project.StatusBar, getThemeColor(GUI_INSIDE))
			rl.DrawLine(int32(project.StatusBar.X), int32(project.StatusBar.Y-1), int32(project.StatusBar.X+project.StatusBar.Width), int32(project.StatusBar.Y-1), getThemeColor(GUI_OUTLINE))

			taskCount := 0
			completionCount := 0

			for _, t := range project.CurrentBoard().Tasks {

				taskCount++
				if t.IsComplete() {
					completionCount++
				}

			}

			percentage := int32(0)
			if taskCount > 0 && completionCount > 0 {
				percentage = int32(float32(completionCount) / float32(taskCount) * 100)
			}

			DrawGUIText(rl.Vector2{6, project.StatusBar.Y - 2}, "%d / %d Tasks completed (%d%%)", completionCount, taskCount, percentage)

			PrevMousePosition = GetMousePosition()

			todayText := time.Now().Format("Monday, January 2, 2006, 15:04:05")
			textLength := rl.MeasureTextEx(guiFont, todayText, guiFontSize, spacing)
			pos := rl.Vector2{float32(rl.GetScreenWidth())/2 - textLength.X/2, project.StatusBar.Y - 2}
			pos.X = float32(int(pos.X))
			pos.Y = float32(int(pos.Y))

			DrawGUIText(pos, todayText)

			// Search bar

			project.Searchbar.Rect.Y = project.StatusBar.Y + 1
			project.Searchbar.Rect.X = float32(rl.GetScreenWidth()) - (project.Searchbar.Rect.Width + 16)

			rl.DrawTextureRec(project.GUI_Icons, rl.Rectangle{128, 0, 16, 16}, rl.Vector2{project.Searchbar.Rect.X - 24, project.Searchbar.Rect.Y + 4}, getThemeColor(GUI_OUTLINE_HIGHLIGHTED))

			clickedOnSearchbar := false

			searchbarWasFocused := project.Searchbar.Focused

			project.Searchbar.Update()

			if project.Searchbar.Focused && !searchbarWasFocused {
				clickedOnSearchbar = true
			}

			if project.Searchbar.Text() != "" {

				if project.Searchbar.Changed || clickedOnSearchbar {
					project.SearchForTasks()
				}

				searchTextPosX := project.Searchbar.Rect.X - 96
				searchCount := "0/0"
				if len(project.SearchedTasks) > 0 {
					searchCount = fmt.Sprintf("%d / %d", project.FocusedSearchTask+1, len(project.SearchedTasks))
				}
				textMeasure := rl.MeasureTextEx(guiFont, searchCount, guiFontSize, spacing)
				textMeasure.X = float32(int(textMeasure.X / 2))
				textMeasure.Y = float32(int(textMeasure.Y / 2))

				if ImmediateButton(rl.Rectangle{searchTextPosX - textMeasure.X - 28, project.Searchbar.Rect.Y, project.Searchbar.Rect.Height, project.Searchbar.Rect.Height}, "<", len(project.SearchedTasks) == 0) {
					project.FocusedSearchTask--
					project.SearchForTasks()
				}

				DrawGUIText(rl.Vector2{searchTextPosX - textMeasure.X, project.Searchbar.Rect.Y - 2}, searchCount)

				if ImmediateButton(rl.Rectangle{searchTextPosX + textMeasure.X + 12, project.Searchbar.Rect.Y, project.Searchbar.Rect.Height, project.Searchbar.Rect.Height}, ">", len(project.SearchedTasks) == 0) {
					project.FocusedSearchTask++
					project.SearchForTasks()
				}

			}

			// Boards

			w := float32(0)
			for _, b := range currentProject.Boards {
				bw := GUITextWidth(b.Name)
				if bw > w {
					w = bw
				}
			}

			if 112 > w {
				w = 112
			}

			w += 32 // Make room for the icon

			y := float32(64)
			buttonRange := float32(72)
			x := float32(rl.GetScreenWidth()-int(w)) - buttonRange - 64
			h := float32(24)
			iconSrcRect := rl.Rectangle{96, 16, 16, 16}

			project.BoardPanel = rl.Rectangle{x, y, w + 100, h * float32(len(project.Boards)+1)}

			if !project.TaskOpen {

				for boardIndex, board := range project.Boards {

					disabled := boardIndex == project.BoardIndex

					if len(project.Boards[boardIndex].Tasks) == 0 {
						iconSrcRect.X += iconSrcRect.Width
					}

					if ImmediateIconButton(rl.Rectangle{x + buttonRange, y, w, h}, iconSrcRect, 0, board.Name, disabled) {

						project.BoardIndex = boardIndex
						project.Log("Switched to Board: %s.", board.Name)

					}

					if disabled {

						bx := x + buttonRange - h
						if ImmediateIconButton(rl.Rectangle{bx, y, h, h}, rl.Rectangle{16, 16, 16, 16}, 90, "", boardIndex == len(project.Boards)-1) {
							b := project.Boards[boardIndex+1]
							project.Boards[boardIndex] = b
							project.Boards[boardIndex+1] = board
							project.BoardIndex++
							project.Log("Moved Board %s down.", board.Name)
						}
						bx -= h
						if ImmediateIconButton(rl.Rectangle{bx, y, h, h}, rl.Rectangle{16, 16, 16, 16}, -90, "", boardIndex == 0) {
							b := project.Boards[boardIndex-1]
							project.Boards[boardIndex] = b
							project.Boards[boardIndex-1] = board
							project.BoardIndex--
							project.Log("Moved Board %s up.", board.Name)
						}
						bx -= h
						if ImmediateIconButton(rl.Rectangle{bx, y, h, h}, rl.Rectangle{160, 16, 16, 16}, 0, "", false) {
							project.RenameBoardPopup.Textbox.SetText(project.CurrentBoard().Name)
							project.ActivatePopup(project.RenameBoardPopup)
						}

					}

					y += float32(h)

				}

				if ImmediateButton(rl.Rectangle{x + buttonRange, y, w, h}, "+", false) {
					if project.GetEmptyBoard() != nil {
						project.Log("Can't create new Board while an empty Board exists.")
					} else {
						project.AddBoard()
						project.BoardIndex = len(project.Boards) - 1
						project.Log("New Board %d created.", len(project.Boards)-1)
					}
				}

				empty := project.GetEmptyBoard()
				if empty != nil && empty != project.CurrentBoard() {
					project.RemoveBoard(empty)
				}

				if project.BoardIndex >= len(project.Boards) {
					project.BoardIndex = len(project.Boards) - 1
				}

			}

		}

		if project.AutoSave.Checked && !project.TaskOpen && (rl.IsMouseButtonReleased(rl.MouseMiddleButton) || rl.GetMouseWheelMove() != 0) { // Zooming and panning are also recorded
			project.Save()
		}

	}

}

func (project *Project) GetEmptyBoard() *Board {
	for _, b := range project.Boards {
		if len(b.Tasks) == 0 {
			return b
		}
	}
	return nil
}

func (project *Project) AddBoard() {
	project.Boards = append(project.Boards, NewBoard(project))
}

func (project *Project) RemoveBoard(board *Board) {
	for index, b := range project.Boards {
		if b == board {
			b.Destroy()
			project.Boards = append(project.Boards[:index], project.Boards[index+1:]...)
			project.Log("Deleted empty Board: %s", b.Name)
			break
		}
	}
}

func (project *Project) SearchForTasks() {

	project.SendMessage(MessageSelect, nil)
	project.SearchedTasks = []*Task{}

	if project.Searchbar.Changed {
		project.FocusedSearchTask = 0
	}

	for _, task := range project.GetAllTasks() {

		searchText := strings.ToLower(project.Searchbar.Text())

		resourceTask := task.TaskType.CurrentChoice == TASK_TYPE_IMAGE || task.TaskType.CurrentChoice == TASK_TYPE_SOUND

		if searchText != "" && (strings.Contains(strings.ToLower(task.Description.Text()), searchText) ||
			(resourceTask && strings.Contains(strings.ToLower(task.FilePathTextbox.Text()), searchText)) ||
			(task.TaskType.CurrentChoice == TASK_TYPE_TIMER && strings.Contains(strings.ToLower(task.TimerName.Text()), searchText))) {
			project.SearchedTasks = append(project.SearchedTasks, task)
		}

	}

	if len(project.SearchedTasks) == 0 {
		project.FocusedSearchTask = 0
	} else {
		if project.FocusedSearchTask < 0 {
			project.FocusedSearchTask = len(project.SearchedTasks) - 1
		} else if project.FocusedSearchTask >= len(project.SearchedTasks) {
			project.FocusedSearchTask = 0
		}
	}

	if len(project.SearchedTasks) > 0 {
		task := project.SearchedTasks[project.FocusedSearchTask]
		project.BoardIndex = task.Board.Index()
		project.SendMessage(MessageSelect, map[string]interface{}{"task": task})
		project.CurrentBoard().FocusViewOnSelectedTasks()
	}

}

func (project *Project) GetFirstFreeID() int {

	usedIDs := map[int]bool{}

	tasks := project.GetAllTasks()

	for i := 0; i < firstFreeTaskID; i++ {
		if len(tasks) > i {
			usedIDs[tasks[i].ID] = true
		}
	}

	// Reuse already spent, but nonexistent IDs (i.e. create a task that has ID 4, then
	// delete that and create a new one; it should have an ID of 4 so that when VCS diff
	// the project file, it just alters the relevant pieces of info to make the original
	// Task #4 the new Task #4)
	for i := 0; i < firstFreeTaskID; i++ {
		exists := usedIDs[i]
		if !exists {
			return i
		}
	}

	// If no spent but unused IDs exist, then we can just use a new one and move on.
	id := firstFreeTaskID

	firstFreeTaskID++

	return id

}

func (project *Project) LockPositionToGrid(x, y float32) (float32, float32) {

	return float32(math.Round(float64(x/float32(project.GridSize)))) * float32(project.GridSize),
		float32(math.Round(float64(y/float32(project.GridSize)))) * float32(project.GridSize)

}

func (project *Project) GenerateGrid() {

	data := []byte{}

	for y := int32(0); y < project.GridSize*2; y++ {
		for x := int32(0); x < project.GridSize*2; x++ {

			c := getThemeColor(GUI_INSIDE_DISABLED)
			if project.GridVisible.Checked && (x%project.GridSize == 0 || x%project.GridSize == project.GridSize-1) && (y%project.GridSize == 0 || y%project.GridSize == project.GridSize-1) {
				c = getThemeColor(GUI_INSIDE)
			}

			data = append(data, c.R, c.G, c.B, c.A)
		}
	}

	img := rl.NewImage(data, project.GridSize*2, project.GridSize*2, 1, rl.UncompressedR8g8b8a8)

	if project.GridTexture.ID != 0 {
		rl.UnloadTexture(project.GridTexture)
	}

	project.GridTexture = rl.LoadTextureFromImage(img)

}

func (project *Project) ReloadThemes() {

	loadThemes()

	_, themeExists := guiColors[currentTheme]
	if !themeExists {
		for k := range guiColors {
			currentTheme = k
			project.ColorThemeSpinner.SetChoice(k)
			break
		}
	}

	project.GenerateGrid()
	guiThemes := []string{}
	for theme, _ := range guiColors {
		guiThemes = append(guiThemes, theme)
	}
	sort.Strings(guiThemes)
	project.ColorThemeSpinner.Options = guiThemes

}

func (project *Project) GetFrameTime() float32 {
	ft := rl.GetFrameTime()
	if ft > (1/float32(TARGET_FPS))*2 {
		// This artificial limiting is done to ensure the delta time never gets so high that it makes major problems.
		ft = (1 / float32(TARGET_FPS)) * 2
	}
	return ft
}

func (project *Project) Destroy() {

	for _, board := range project.Boards {
		board.Destroy()
	}

	for filepath, res := range project.Resources {

		if res.IsTexture() {
			rl.UnloadTexture(res.Texture())
		}
		// GIFs don't need to be disposed of directly here; the file handle was already Closed.
		// Audio streams are closed by the Task, as each Sound Task has its own stream.

		if res.Temporary {
			os.Remove(filepath)
		}

	}

}

func (project *Project) LoadResource(resourcePath string) (*Resource, bool) {

	downloadedFile := false
	newlyLoaded := false

	var loadedResource *Resource

	existingResource, exists := project.Resources[resourcePath]

	if exists {
		loadedResource = existingResource
	} else if resourcePath != "" {

		localFilepath := resourcePath

		// Attempt downloading it if it's an HTTP file
		if strings.HasPrefix(resourcePath, "http://") || strings.HasPrefix(resourcePath, "https://") {

			response, err := http.Get(resourcePath)

			if err != nil {

				log.Println("Could not open HTTP address: ", err)
				project.Log("Could not open HTTP address: ", err.Error())

			} else {

				defer response.Body.Close()

				tempFile, err := ioutil.TempFile("", "masterplan_resource")
				defer tempFile.Close()
				if err != nil {
					log.Println(err)
				} else {
					io.Copy(tempFile, response.Body)
					newlyLoaded = true
					localFilepath = tempFile.Name()
					downloadedFile = true
				}

			}

		}

		fileType, err := mimetype.DetectFile(localFilepath)

		if err != nil {
			log.Println("Error identifying file: %s", err.Error())
		} else {

			// We have to rename the resource according to what it is because raylib expects the extensions of files to be correct.
			// png image files need to have .png as an extension, for example.
			if filepath.Ext(localFilepath) != fileType.Extension() {
				newName := localFilepath + fileType.Extension()
				os.Rename(localFilepath, newName)
				localFilepath = newName
			}

			if strings.Contains(fileType.String(), "image") {

				if strings.Contains(fileType.String(), "gif") {
					file, err := os.Open(localFilepath)
					if err != nil {
						log.Println("Could not open GIF: ", err.Error())
					} else {

						defer file.Close()

						gifFile, err := gif.DecodeAll(file)

						if err != nil {
							log.Println("Could not decode GIF: ", err.Error())
						} else {
							res := RegisterResource(resourcePath, localFilepath, gifFile)
							res.Temporary = downloadedFile
							loadedResource = res
						}

					}
				} else { // Ordinary image
					res := RegisterResource(resourcePath, localFilepath, rl.LoadTexture(localFilepath))
					res.Temporary = downloadedFile
					loadedResource = res
				}

			} else if strings.Contains(fileType.String(), "audio") {
				res := RegisterResource(resourcePath, localFilepath, nil)
				res.Temporary = downloadedFile
				loadedResource = res
			}

		}

	}

	return loadedResource, newlyLoaded

}

func (project *Project) WorldToGrid(worldX, worldY float32) (int, int) {
	return int(worldX / float32(project.GridSize)), int(worldY / float32(project.GridSize))
}

func (project *Project) ActivatePopup(popup Popup) {
	project.ActivePopup = popup
	popup.Open()
}

func (project *Project) ExecuteDestructiveAction(action string, argument string) {

	switch action {
	case ActionNewProject:
		project.Destroy()
		currentProject = NewProject()
	case ActionLoadProject:
		project.Destroy()
		currentProject = NewProject()
		if argument == "" {
			currentProject.LoadFrom()
		} else {
			currentProject.Load(argument)
		}
	case ActionSaveAsProject:
		project.FilePath = argument
		project.Save()
	}

}
