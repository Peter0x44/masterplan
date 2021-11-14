// Erase the space before "go" to enable generating the version info from the version info file when it's in the root directory
//go:generate goversioninfo -64=true
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/blang/semver"
	"github.com/cavaliercoder/grab"
	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"github.com/hako/durafmt"
	"github.com/ncruces/zenity"
	"github.com/pkg/browser"
	"github.com/veandco/go-sdl2/gfx"
	"github.com/veandco/go-sdl2/img"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// Build-time variable
var releaseMode = "false"
var demoMode = "" // If set to something other than "", it's a demo

var drawFPS = false
var takeScreenshot = false

var windowTitle = "MasterPlan"
var quit = false
var targetFPS = 60

var cpuProfileStart = time.Time{}

func init() {

	if releaseMode == "true" {

		// Redirect STDERR and STDOUT to log.txt in release mode

		existingLogs := []string{}

		for _, file := range FilesInDirectory(filepath.Join(xdg.ConfigHome, "MasterPlan"), "log") {
			existingLogs = append(existingLogs, file)
		}

		// Destroy old logs; max is 20 (for now)
		for len(existingLogs) > 20 {
			os.Remove(existingLogs[0])
			existingLogs = existingLogs[1:]
		}

		logPath, err := xdg.ConfigFile("MasterPlan/log_" + time.Now().Format(FileTimeFormat) + ".txt")
		if err != nil {
			panic(err)
		}
		f, err := os.Create(logPath)
		if err != nil {
			panic(err)
		}

		os.Stderr = f
		os.Stdout = f

		log.SetOutput(f)

	}

	runtime.LockOSThread()

	globals.Version = semver.MustParse("0.8.0-alpha.2")
	globals.Keyboard = NewKeyboard()
	globals.Mouse = NewMouse()
	nm := NewMouse()
	nm.screenPosition.X = -99999999999
	nm.screenPosition.Y = -99999999999
	nm.prevPosition.X = -99999999999
	nm.prevPosition.Y = -99999999999
	globals.Mouse.Dummy = &nm
	globals.Resources = NewResourceBank()
	globals.GridSize = 32
	globals.InputText = []rune{}
	globals.CopyBuffer = NewCopyBuffer()
	globals.State = StateNeutral
	globals.GrabClient = grab.NewClient()
	globals.MenuSystem = NewMenuSystem()
	globals.Keybindings = NewKeybindings()
	globals.Settings = NewProgramSettings()

}

func main() {

	// We want to defer a function to recover out of a crash if in release mode.
	// We do this because by default, Go's stderr points directly to the OS's syserr buffer.
	// By deferring this function and recovering out of the crash, we can grab the crashlog by
	// using runtime.Caller().

	defer func() {
		if releaseMode == "true" {
			panicOut := recover()
			if panicOut != nil {

				log.Print(
					"# ERROR START #\n",
				)

				stackContinue := true
				i := 0 // We can skip the first few crash lines, as they reach up through the main
				// function call and into this defer() call.
				for stackContinue {
					// Recover the lines of the crash log and log it out.
					_, fn, line, ok := runtime.Caller(i)
					stackContinue = ok
					if ok {
						fmt.Print("\n", fn, ":", line)
						if i == 0 {
							fmt.Print(" | ", "Error: ", panicOut)
						}
						i++
					}
				}

				fmt.Print(
					"\n\n# ERROR END #\n",
				)
			}
		}
	}()

	// settingsLoaded := globals.ProgramSettings.Load()

	settingsLoaded := true

	fmt.Println("Release mode:", releaseMode)

	loadThemes()

	if demoMode != "" {
		demoMode = " " + demoMode
	}

	// windowFlags := byte(rl.FlagWindowResizable)

	// if programSettings.BorderlessWindow {
	// 	windowFlags += rl.FlagWindowUndecorated
	// }

	// if programSettings.TransparentBackground {
	// 	windowFlags += rl.FlagWindowTransparent
	// }

	// rl.SetConfigFlags(windowFlags)

	// // We initialize the window using just "MasterPlan" as the title because WM_CLASS is set from this on Linux
	// rl.InitWindow(960, 540, "MasterPlan")

	// rl.SetWindowIcon(*rl.LoadImage(LocalPath("assets", "window_icon.png")))

	// if programSettings.SaveWindowPosition && programSettings.WindowPosition.Width > 0 && programSettings.WindowPosition.Height > 0 {
	// 	rl.SetWindowPosition(int(programSettings.WindowPosition.X), int(programSettings.WindowPosition.Y))
	// 	rl.SetWindowSize(int(programSettings.WindowPosition.Width), int(programSettings.WindowPosition.Height))
	// }

	x := int32(sdl.WINDOWPOS_UNDEFINED)
	y := int32(sdl.WINDOWPOS_UNDEFINED)
	w := int32(960)
	h := int32(540)

	if globals.Settings.Get(SettingsSaveWindowPosition).AsBool() && globals.Settings.Has(SettingsWindowPosition) {
		windowData := globals.Settings.Get(SettingsWindowPosition).AsMap()
		x = int32(windowData["X"].(float64))
		y = int32(windowData["Y"].(float64))
		w = int32(windowData["W"].(float64))
		h = int32(windowData["H"].(float64))
	}

	windowFlags := uint32(sdl.WINDOW_RESIZABLE)

	if globals.Settings.Get(SettingsBorderlessWindow).AsBool() {
		windowFlags |= sdl.WINDOW_BORDERLESS
	}

	if err := ttf.Init(); err != nil {
		panic(err)
	}

	if err := speaker.Init(beep.SampleRate(44100), 2048); err != nil {
		panic(err)
	}

	// window, renderer, err := sdl.CreateWindowAndRenderer(w, h, windowFlags)
	window, err := sdl.CreateWindow("MasterPlan", sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED, w, h, windowFlags)
	if err != nil {
		panic(err)
	}

	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "2")

	// Should default to hardware accelerators, if available
	renderer, err := sdl.CreateRenderer(window, 0, sdl.RENDERER_ACCELERATED+sdl.RENDERER_SOFTWARE)
	if err != nil {
		panic(err)
	}

	// if globals.ProgramSettings.Get(SettingsSaveWindowPosition).AsBool() && globals.OldProgramSettings.WindowPosition.W > 0 && globals.OldProgramSettings.WindowPosition.H > 0 {
	// 	x = int32(globals.OldProgramSettings.WindowPosition.X)
	// 	y = int32(globals.OldProgramSettings.WindowPosition.Y)
	// 	w = int32(globals.OldProgramSettings.WindowPosition.W)
	// 	h = int32(globals.OldProgramSettings.WindowPosition.H)
	// }

	LoadCursors()

	icon, err := img.Load(LocalRelativePath("assets/window_icon.png"))
	if err != nil {
		panic(err)
	}
	window.SetIcon(icon)
	window.SetPosition(x, y)
	window.SetSize(w, h)
	sdl.SetHint(sdl.HINT_VIDEO_MINIMIZE_ON_FOCUS_LOSS, "0")
	sdl.SetHint(sdl.HINT_RENDER_BATCHING, "1")
	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "0")

	globals.Window = window
	globals.Renderer = renderer
	globals.TextRenderer = NewTextRenderer()
	screenWidth, screenHeight, _ := globals.Renderer.GetOutputSize()
	globals.ScreenSize = Point{float32(screenWidth), float32(screenHeight)}
	globals.EventLog = NewEventLog()

	globals.TriggerReloadFonts = true
	HandleFontReload()

	globals.Project = NewProject()

	ConstructMenus()

	// renderer.SetLogicalSize(960, 540)

	attemptAutoload := 5
	showedAboutDialog := false
	splashScreenTime := float32(0)
	// splashScreen := rl.LoadTexture(LocalPath("assets", "splashscreen.png"))
	splashColor := sdl.Color{255, 255, 255, 255}

	if globals.Settings.Get(SettingsDisableSplashscreen).AsBool() {
		splashScreenTime = 100
		splashColor.A = 0
	}

	fpsManager := &gfx.FPSmanager{}

	gfx.InitFramerate(fpsManager)
	gfx.SetFramerate(fpsManager, 60)

	// fpsDisplayValue := float32(0)
	// fpsDisplayAccumulator := float32(0)
	// fpsDisplayTimer := time.Now()

	log.Println("MasterPlan initialized successfully.")

	// go func() {

	// 	for {
	// 		fmt.Println(fpsDisplayValue)
	// 		time.Sleep(time.Second)
	// 	}

	// }()

	fullscreen := false

	for !quit {

		globals.MenuSystem.Get("main").Pages["root"].FindElement("time label", false).(*Label).SetText([]rune(time.Now().Format("Mon Jan 2 2006")))

		screenWidth, screenHeight, err := globals.Renderer.GetOutputSize()

		if err != nil {
			panic(err)
		}

		globals.ScreenSize = Point{float32(screenWidth), float32(screenHeight)}

		globals.Time += float64(globals.DeltaTime)

		if globals.Frame == math.MaxInt64 {
			globals.Frame = 0
		}
		globals.Frame++

		handleEvents()

		// currentTime := time.Now()

		// handleMouseInputs()

		// if globals.ProgramSettings.Keybindings.On(KBShowFPS) {
		// 	drawFPS = !drawFPS
		// }

		if globals.Keybindings.Pressed(KBWindowSizeSmall) {
			window.SetSize(960, 540)
		}

		if globals.Keybindings.Pressed(KBWindowSizeNormal) {
			window.SetSize(1920, 1080)
		}

		if globals.Keybindings.Pressed(KBToggleFullscreen) {
			fullscreen = !fullscreen
			if fullscreen {
				window.SetFullscreen(sdl.WINDOW_FULLSCREEN_DESKTOP)
			} else {
				window.SetFullscreen(0)
			}
		}

		globals.WindowFlags = window.GetFlags()
		windowFocused := globals.WindowFlags&sdl.WINDOW_MINIMIZED == 0 && globals.WindowFlags&sdl.WINDOW_HIDDEN == 0

		// if windowFlags&byte(rl.FlagWindowTransparent) > 0 {
		// 	clearColor = rl.Color{}
		// }
		clearColor := getThemeColor(GUIBGColor)
		renderer.SetDrawColor(clearColor.RGBA())
		renderer.Clear()

		if attemptAutoload > 0 {

			attemptAutoload--

			if attemptAutoload == 0 {

				// If the settings aren't successfully loaded, it's safe to assume it's because they don't exist, because the program is first loading.
				if !settingsLoaded {

					// if loaded := LoadProject(LocalPath("assets", "help_manual.plan")); loaded != nil {
					// 	currentProject = loaded
					// }

				} else {

					//Loads file when passed in as argument; courtesy of @DanielKilgallon on GitHub.

					// var loaded *Project

					// if len(os.Args) > 1 {
					// 	loaded = LoadProject(os.Args[1])
					// } else if programSettings.AutoloadLastPlan && len(programSettings.RecentPlanList) > 0 {
					// 	loaded = LoadProject(programSettings.RecentPlanList[0])
					// }

					// if loaded != nil {
					// 	currentProject = loaded
					// }

				}

			}

		} else {

			// fmt.Println(globals.TextRenderer.MeasureText([]rune("New Project"), 1))

			// if globals.State == StateNeutral && globals.Keybindings.Pressed(KBDebugRestart) {
			// 	globals.Project = NewProject()
			// }

			if globals.Keybindings.Pressed(KBDebugToggle) {
				globals.DebugMode = !globals.DebugMode
			}

			if globals.Keyboard.Key(sdl.K_F5).Pressed() {
				profileCPU()
			}

			// if rl.WindowShouldClose() {
			// 	currentProject.PromptQuit()
			// }

			if !showedAboutDialog {
				showedAboutDialog = true
				if globals.Settings.Get(SettingsShowAboutDialogOnStart).AsBool() {
					settings := globals.MenuSystem.Get("settings")
					settings.Center()
					settings.Open()
					settings.SetPage("about")
				}
			}

			globals.MenuSystem.Update()

			globals.Project.Update()

			globals.Keybindings.On = true

			if windowFocused {

				globals.Project.Draw()

				globals.Renderer.SetScale(1, 1)

				globals.MenuSystem.Draw()

				if globals.DebugMode {
					fps, _ := gfx.GetFramerate(fpsManager)
					s := strconv.FormatFloat(float64(fps), 'f', 0, 64)
					globals.TextRenderer.QuickRenderText(s, Point{globals.ScreenSize.X - 64, 0}, 1, ColorWhite, AlignRight)
				}

			}

			if globals.Project.LoadingProject != nil {
				original := globals.Project
				loading := globals.Project.LoadingProject
				globals.Project = loading
				original.Destroy()
			}

			// y := int32(0)

			// for _, event := range eventLogBuffer {
			// 	src := &sdl.Rect{0, 0, int32(event.Texture.Image.Size.X), int32(event.Texture.Image.Size.Y)}
			// 	dst := &sdl.Rect{0, y, int32(event.Texture.Image.Size.X), int32(event.Texture.Image.Size.Y)}
			// 	globals.Renderer.Copy(event.Texture.Image.Texture, src, dst)
			// 	y += src.H
			// }

			// rl.EndMode2D()

			// color := getThemeColor(GUI_FONT_COLOR)
			// color.A = 128

			// x := float32(0)
			// // x := float32(rl.GetScreenWidth() - 8)
			// v := ""

			// if currentProject.LockProject.Checked {
			// 	if currentProject.Locked {
			// 		v += "Project Lock Engaged"
			// 	} else {
			// 		v += "Project Lock Present"
			// 	}
			// } else if currentProject.AutoSave.Checked {
			// 	if currentProject.FilePath == "" {
			// 		v += "Please Manually Save Project"
			// 		color.R = 255
			// 	} else {
			// 		v += "Autosave On"
			// 	}
			// } else if currentProject.Modified {
			// 	v += "Modified"
			// }

			// if len(v) > 0 {
			// 	size, _ := TextSize(v, true)
			// 	x -= size.X
			// 	// DrawGUITextColored(rl.Vector2{x, 8}, color, v)
			// }

			// color = rl.White
			// bgColor := rl.Black

			// y := float32(24)

			msgSize := float32(1)
			eventY := globals.ScreenSize.Y

			for _, event := range globals.EventLog.Events {

				bgColor := getThemeColor(GUIMenuColor)
				fontColor := getThemeColor(GUIFontColor)
				fadeValue, _, _ := event.Tween.Update(globals.DeltaTime)

				if globals.Settings.Get(SettingsDisplayMessages).AsBool() {

					event.Y += (eventY - event.Y) * 0.2

					fade := uint8(float32(fontColor[3]) * fadeValue)

					textSize := globals.TextRenderer.MeasureText([]rune(event.Text), msgSize)

					dst := &sdl.FRect{0, event.Y, textSize.X, textSize.Y}
					bgColor[3] = fade
					fontColor[3] = fade

					FillRect(dst.X, dst.Y-dst.H, dst.W, dst.H, bgColor)
					globals.TextRenderer.QuickRenderText(event.Text, Point{0, event.Y - dst.H}, msgSize, fontColor, AlignLeft)

					eventY -= dst.H

				}

			}

			globals.EventLog.CleanUpDeadEvents()

			// if !programSettings.DisableMessageLog {

			// 	for i := 0; i < len(eventLogBuffer); i++ {

			// 		msg := eventLogBuffer[i]

			// 		text := "- " + msg.Time.Format("15:04:05") + " : " + msg.Text
			// 		text = strings.ReplaceAll(text, "\n", "\n                    ")

			// 		alpha, done := msg.Tween.Update(1 / float32(programSettings.TargetFPS))

			// 		if strings.HasPrefix(msg.Text, "ERROR") {
			// 			color = rl.Red
			// 		} else if strings.HasPrefix(msg.Text, "WARNING") {
			// 			color = rl.Yellow
			// 		} else {
			// 			color = rl.White
			// 		}

			// 		color.A = uint8(alpha)
			// 		bgColor.A = color.A

			// 		textSize := rl.MeasureTextEx(font, text, float32(GUIFontSize()), 1)
			// 		lineHeight, _ := TextHeight(text, true)
			// 		textPos := rl.Vector2{8, y}
			// 		rectPos := textPos

			// 		rectPos.X--
			// 		rectPos.Y--
			// 		textSize.X += 2
			// 		textSize.Y = lineHeight

			// 		rl.DrawRectangleV(textPos, textSize, bgColor)
			// 		DrawGUITextColored(textPos, color, text)

			// 		if done {
			// 			eventLogBuffer = append(eventLogBuffer[:i], eventLogBuffer[i+1:]...)
			// 			i--
			// 		}

			// 		y += lineHeight

			// 	}

			// }

			// if globals.ProgramSettings.Keybindings.On(KBTakeScreenshot) {
			// 	// This is here because you can trigger a screenshot from the context menu as well.
			// 	takeScreenshot = true
			// }

			// if takeScreenshot {
			// 	// Use the current time for screenshot names; ".00" adds the fractional second
			// 	screenshotFileName := fmt.Sprintf("screenshot_%s.png", time.Now().Format(FileTimeFormat+".00"))
			// 	screenshotPath := LocalPath(screenshotFileName)
			// 	if projectScreenshotsPath := currentProject.ScreenshotsPath.Text(); projectScreenshotsPath != "" {
			// 		if _, err := os.Stat(projectScreenshotsPath); err == nil {
			// 			screenshotPath = filepath.Join(projectScreenshotsPath, screenshotFileName)
			// 		}
			// 	}
			// 	rl.TakeScreenshot(screenshotPath)
			// 	currentProject.Log("Screenshot saved successfully to %s.", screenshotPath)
			// 	takeScreenshot = false
			// }
		}

		splashScreenTime += globals.DeltaTime

		if splashScreenTime >= 0.5 {
			sub := uint8(255 * globals.DeltaTime * 4)
			if splashColor.A > sub {
				splashColor.A -= sub
			} else {
				splashColor.A = 0
			}
		}

		// if splashColor.A > 0 {
		// 	src := rl.Rectangle{0, 0, float32(splashScreen.Width), float32(splashScreen.Height)}
		// 	dst := rl.Rectangle{0, 0, float32(rl.GetScreenWidth()), float32(rl.GetScreenHeight())}
		// 	rl.DrawTexturePro(splashScreen, src, dst, rl.Vector2{}, 0, splashColor)
		// }

		renderer.Present()

		title := "MasterPlan v" + globals.Version.String() + demoMode

		// if currentProject.FilePath != "" {
		// 	_, fileName := filepath.Split(currentProject.FilePath)
		// 	title += fmt.Sprintf(" - %s", fileName)
		// }

		// if currentProject.Modified {
		// 	title += " *"
		// }

		if windowTitle != title {
			window.SetTitle(title)
			windowTitle = title
		}

		newTarget := int(globals.Settings.Get(SettingsTargetFPS).AsFloat())

		if globals.WindowFlags&sdl.WINDOW_INPUT_FOCUS == 0 {
			newTarget = int(globals.Settings.Get(SettingsUnfocusedFPS).AsFloat())
		} else if !windowFocused {
			newTarget = 5 // Automatically drop to 5 FPS if the window's minimized
		}

		if newTarget <= 0 {
			newTarget = 5
		}

		if targetFPS != newTarget {
			targetFPS = newTarget
			gfx.SetFramerate(fpsManager, uint32(targetFPS))
		}

		dt := float32(gfx.FramerateDelay(fpsManager)) / 1000

		if dt > 0 && dt < float32(math.Inf(1)) {
			globals.DeltaTime = dt
		}

		HandleFontReload()

	}

	if globals.Settings.Get(SettingsSaveWindowPosition).AsBool() {
		// This is outside the main loop because we can save the window properties just before quitting
		wX, wY := window.GetPosition()
		wW, wH := window.GetSize()
		globals.Settings.Get(SettingsWindowPosition).Set(sdl.Rect{wX, wY, wW, wH})
	}

	log.Println("MasterPlan exited successfully.")

	globals.Project.Destroy()

	globals.Resources.Destroy()

	sdl.Quit()

}

func ConstructMenus() {

	// Main Menu

	mainMenu := globals.MenuSystem.Add(NewMenu(&sdl.FRect{0, 0, 800, 48}, false), "main", false)
	root := mainMenu.Pages["root"]

	row := root.AddRow(AlignCenter)
	row.Add("file menu", NewButton("File", &sdl.FRect{0, 0, 96, 32}, &sdl.Rect{144, 0, 32, 32}, false, func() {
		globals.MenuSystem.Get("file").Open()
	}))

	row.Add("view menu", NewButton("View", &sdl.FRect{0, 0, 96, 32}, nil, false, func() {
		globals.MenuSystem.Get("view").Open()
	}))

	row.Add("spacer", NewSpacer(&sdl.FRect{0, 0, 256, 32}))
	row.Add("time label", NewLabel(time.Now().Format("Mon Jan 2 2006"), &sdl.FRect{0, 0, 256, 32}, false, AlignCenter))

	// File Menu

	fileMenu := globals.MenuSystem.Add(NewMenu(&sdl.FRect{0, 48, 300, 300}, true), "file", false)
	root = fileMenu.Pages["root"]

	root.AddRow(AlignCenter).Add("New Project", NewButton("New Project", nil, nil, false, func() {
		globals.Project.LoadingProject = NewProject()
		globals.EventLog.Log("New project created.")
	}))
	root.AddRow(AlignCenter).Add("Load Project", NewButton("Load Project", nil, nil, false, func() { globals.Project.Open() }))
	root.AddRow(AlignCenter).Add("Save Project", NewButton("Save Project", nil, nil, false, func() {
		if globals.Project.Filepath != "" {
			globals.Project.Save()
		} else {
			globals.Project.SaveAs()
		}
	}))
	root.AddRow(AlignCenter).Add("Save Project As...", NewButton("Save Project As...", &sdl.FRect{0, 0, 256, 32}, nil, false, func() { globals.Project.SaveAs() }))
	root.AddRow(AlignCenter).Add("Settings", NewButton("Settings", nil, nil, false, func() {
		settings := globals.MenuSystem.Get("settings")
		settings.Center()
		settings.Open()
		fileMenu.Close()
	}))
	root.AddRow(AlignCenter).Add("Quit", NewButton("Quit", nil, nil, false, func() {
		confirmQuit := globals.MenuSystem.Get("confirmquit")
		confirmQuit.Center()
		confirmQuit.Open()
		fileMenu.Close()
	}))

	// View Menu

	viewMenu := globals.MenuSystem.Add(NewMenu(&sdl.FRect{48, 48, 300, 200}, true), "view", false)
	root = viewMenu.Pages["root"]

	root.AddRow(AlignCenter).Add("Create Menu", NewButton("Create", nil, nil, false, func() {
		globals.MenuSystem.Get("create").Open()
		viewMenu.Close()
	}))

	root.AddRow(AlignCenter).Add("Edit Menu", NewButton("Edit", nil, nil, false, func() {
		globals.MenuSystem.Get("edit").Open()
		viewMenu.Close()
	}))

	// root.AddRow(AlignCenter).Add("Board Menu", NewButton("Boards", nil, nil, false, func() {
	// 	boardMenu := globals.MenuSystem.Get("boards")
	// 	// boardMenu.Center()
	// 	boardMenu.Rect.X = globals.ScreenSize.X - boardMenu.Rect.W
	// 	boardMenu.Rect.Y = 0
	// 	boardMenu.Open()
	// 	viewMenu.Close()
	// }))

	root.AddRow(AlignCenter).Add("Find Menu", NewButton("Find", nil, nil, false, func() {
		globals.MenuSystem.Get("search").Open()
		viewMenu.Close()
	}))

	root.AddRow(AlignCenter).Add("Stats", NewButton("Stats", nil, nil, false, func() {
		globals.MenuSystem.Get("stats").Open()
		viewMenu.Close()
	}))

	// Create Menu

	createMenu := globals.MenuSystem.Add(NewMenu(&sdl.FRect{globals.ScreenSize.X, globals.ScreenSize.Y, 250, 320}, true), "create", false)
	createMenu.Draggable = true
	createMenu.Resizeable = true
	createMenu.CloseButtonEnabled = true
	createMenu.Orientation = MenuOrientationVertical
	createMenu.Open()

	root = createMenu.Pages["root"]
	root.AddRow(AlignCenter).Add("create label", NewLabel("-Create-", &sdl.FRect{0, 0, 128, 32}, false, AlignCenter))

	root.AddRow(AlignCenter).Add("create new checkbox", NewButton("Checkbox", nil, &sdl.Rect{48, 32, 32, 32}, false, func() {
		card := globals.Project.CurrentPage.CreateNewCard(ContentTypeCheckbox)
		card.SetCenter(globals.Project.Camera.TargetPosition)
	}))

	root.AddRow(AlignCenter).Add("create new numbered", NewButton("Numbered", nil, &sdl.Rect{48, 96, 32, 32}, false, func() {
		card := globals.Project.CurrentPage.CreateNewCard(ContentTypeNumbered)
		card.SetCenter(globals.Project.Camera.TargetPosition)
	}))

	root.AddRow(AlignCenter).Add("create new note", NewButton("Note", nil, &sdl.Rect{80, 0, 32, 32}, false, func() {
		card := globals.Project.CurrentPage.CreateNewCard(ContentTypeNote)
		card.SetCenter(globals.Project.Camera.TargetPosition)
	}))

	root.AddRow(AlignCenter).Add("create new sound", NewButton("Sound", nil, &sdl.Rect{80, 32, 32, 32}, false, func() {
		card := globals.Project.CurrentPage.CreateNewCard(ContentTypeSound)
		card.SetCenter(globals.Project.Camera.TargetPosition)
	}))

	root.AddRow(AlignCenter).Add("create new image", NewButton("Image", nil, &sdl.Rect{48, 64, 32, 32}, false, func() {
		card := globals.Project.CurrentPage.CreateNewCard(ContentTypeImage)
		card.SetCenter(globals.Project.Camera.TargetPosition)
	}))

	root.AddRow(AlignCenter).Add("create new timer", NewButton("Timer", nil, &sdl.Rect{80, 64, 32, 32}, false, func() {
		card := globals.Project.CurrentPage.CreateNewCard(ContentTypeTimer)
		card.SetCenter(globals.Project.Camera.TargetPosition)
	}))

	root.AddRow(AlignCenter).Add("create new map", NewButton("Map", nil, &sdl.Rect{112, 96, 32, 32}, false, func() {
		card := globals.Project.CurrentPage.CreateNewCard(ContentTypeMap)
		card.SetCenter(globals.Project.Camera.TargetPosition)
	}))

	// Edit Menu

	editMenu := globals.MenuSystem.Add(NewMenu(&sdl.FRect{globals.ScreenSize.X / 2, globals.ScreenSize.Y / 2, 300, 400}, true), "edit", false)
	editMenu.Draggable = true
	editMenu.Resizeable = true
	editMenu.CloseButtonEnabled = true
	editMenu.Orientation = MenuOrientationVertical

	root = editMenu.Pages["root"]
	root.AddRow(AlignCenter).Add("edit label", NewLabel("-Edit-", &sdl.FRect{0, 0, 128, 32}, false, AlignCenter))
	root.AddRow(AlignCenter).Add("set type", NewButton("Set Type", &sdl.FRect{0, 0, 128, 32}, nil, false, func() {
		editMenu.SetPage("set type")
	}))

	setType := editMenu.AddPage("set type")
	setType.AddRow(AlignCenter).Add("label", NewLabel("Set Type:", &sdl.FRect{0, 0, 192, 32}, false, AlignCenter))

	setType.AddRow(AlignCenter).Add("set checkbox content type", NewButton("Checkbox", nil, &sdl.Rect{48, 32, 32, 32}, false, func() {
		for _, card := range globals.Project.CurrentPage.Selection.AsSlice() {
			card.SetContents(ContentTypeCheckbox)
		}
	}))

	setType.AddRow(AlignCenter).Add("set number content type", NewButton("Number", nil, &sdl.Rect{48, 96, 32, 32}, false, func() {
		for _, card := range globals.Project.CurrentPage.Selection.AsSlice() {
			card.SetContents(ContentTypeNumbered)
		}
	}))

	setType.AddRow(AlignCenter).Add("set note content type", NewButton("Note", nil, &sdl.Rect{80, 0, 32, 32}, false, func() {
		for _, card := range globals.Project.CurrentPage.Selection.AsSlice() {
			card.SetContents(ContentTypeNote)
		}
	}))

	setType.AddRow(AlignCenter).Add("set sound content type", NewButton("Sound", nil, &sdl.Rect{80, 32, 32, 32}, false, func() {
		for _, card := range globals.Project.CurrentPage.Selection.AsSlice() {
			card.SetContents(ContentTypeSound)
		}
	}))

	setType.AddRow(AlignCenter).Add("set image content type", NewButton("Image", nil, &sdl.Rect{48, 64, 32, 32}, false, func() {
		for _, card := range globals.Project.CurrentPage.Selection.AsSlice() {
			card.SetContents(ContentTypeImage)
		}
	}))

	setType.AddRow(AlignCenter).Add("set timer content type", NewButton("Timer", nil, &sdl.Rect{80, 64, 32, 32}, false, func() {
		for _, card := range globals.Project.CurrentPage.Selection.AsSlice() {
			card.SetContents(ContentTypeTimer)
		}
	}))

	setType.AddRow(AlignCenter).Add("set map content type", NewButton("Map", nil, &sdl.Rect{112, 96, 32, 32}, false, func() {
		for _, card := range globals.Project.CurrentPage.Selection.AsSlice() {
			card.SetContents(ContentTypeMap)
		}
	}))

	// Board Menu

	boardMenu := globals.MenuSystem.Add(NewMenu(&sdl.FRect{0, 0, 256, 512}, true), "boards", false)
	boardMenu.CloseButtonEnabled = true
	boardMenu.Resizeable = true
	boardMenu.Draggable = true
	boardMenu.OnOpen = func() {
		root := boardMenu.Pages["root"]
		root.Clear()

		root.AddRow(AlignCenter).Add("title", NewLabel("~Board~", nil, false, AlignCenter))
		root.AddRow(AlignCenter).Add("spacer", NewSpacer(nil))

		row := root.AddRow(AlignCenter)

		row.Add("add page", NewButton("Page", nil, &sdl.Rect{48, 96, 32, 32}, false, func() {
			globals.Project.RootFolder.Add(NewPage(globals.Project.RootFolder, globals.Project))
			boardMenu.Open()
		}))

		row.Add("add folder", NewButton("Folder", nil, &sdl.Rect{48, 96, 32, 32}, false, func() {
			globals.Project.RootFolder.Add(NewPageFolder(globals.Project.RootFolder, globals.Project))
			boardMenu.Open()
		}))

		root.AddRow(AlignCenter).Add("spacer", NewSpacer(nil))

		var createButtonsForBoardMenu func(PageContent)

		createButtonsForBoardMenu = func(element PageContent) {

			localElement := element

			name := ""

			for i := 0; i < localElement.Depth(); i++ {
				name += "   "
			}

			name += localElement.Name()

			if globals.Project.CurrentPage == localElement {
				name = "> " + name
			}

			icon := &sdl.Rect{176, 64, 32, 32}

			if localElement.Type() == PageContentFolder {
				icon.X = 112
			}

			root.AddRow(AlignLeft).Add(name, NewButton(name, nil, icon, false, func() {
				if localElement.Type() == PageContentPage {
					globals.Project.CurrentPage = localElement.(*Page)
					boardMenu.Open()
				} else {
					folder := localElement.(*PageFolder)
					folder.Expanded = !folder.Expanded
					boardMenu.Open()
				}
			}))

			if localElement.Type() == PageContentFolder {

				folder := localElement.(*PageFolder)

				if folder.Expanded {

					for _, content := range localElement.(*PageFolder).Contents {
						// We have to make a local copy so that calling createButtonsForBoardMenu() works in this for loop on each element in the loop
						localContent := content
						createButtonsForBoardMenu(localContent)
					}
				}

			}

		}

		createButtonsForBoardMenu(globals.Project.RootFolder)

	}
	root = boardMenu.Pages["root"]
	root.AddRow(AlignLeft).Add("0", NewButton("Page 1", nil, nil, false, func() {}))

	// Context Menu

	contextMenu := globals.MenuSystem.Add(NewMenu(&sdl.FRect{0, 0, 256, 256}, true), "context", false)
	contextMenu.OnOpen = func() { globals.State = StateContextMenu }
	contextMenu.OnClose = func() { globals.State = StateNeutral }
	root = contextMenu.Pages["root"]

	root.AddRow(AlignCenter).Add("create card", NewButton("Create Card", &sdl.FRect{0, 0, 192, 32}, nil, false, func() {
		globals.Project.CurrentPage.CreateNewCard(ContentTypeCheckbox)
		contextMenu.Close()
	}))

	root.AddRow(AlignCenter).Add("delete cards", NewButton("Delete Cards", &sdl.FRect{0, 0, 192, 32}, nil, false, func() {
		page := globals.Project.CurrentPage
		page.DeleteCards(page.Selection.AsSlice()...)
		contextMenu.Close()
	}))

	root.AddRow(AlignCenter).Add("copy cards", NewButton("Copy Cards", &sdl.FRect{0, 0, 192, 32}, nil, false, func() {
		page := globals.Project.CurrentPage
		page.CopySelectedCards()
		contextMenu.Close()
	}))

	root.AddRow(AlignCenter).Add("paste cards", NewButton("Paste Cards", &sdl.FRect{0, 0, 192, 32}, nil, false, func() {
		page := globals.Project.CurrentPage
		page.PasteCards()
		contextMenu.Close()
	}))

	commonMenu := globals.MenuSystem.Add(NewMenu(&sdl.FRect{globals.ScreenSize.X / 4, globals.ScreenSize.Y/2 - 32, globals.ScreenSize.X / 2, 128}, true), "common", false)
	commonMenu.Draggable = true
	commonMenu.Resizeable = true
	commonMenu.CloseButtonEnabled = true

	// Confirm Quit Menu

	confirmQuit := globals.MenuSystem.Add(NewMenu(&sdl.FRect{0, 0, 32, 32}, true), "confirmquit", true)
	confirmQuit.Draggable = true

	root = confirmQuit.Pages["root"]
	root.AddRow(AlignCenter).Add("label", NewLabel("Are you sure you wish to quit?", nil, false, AlignCenter))
	root.AddRow(AlignCenter).Add("label", NewLabel("Any unsaved changes will be lost.", nil, false, AlignCenter))
	row = root.AddRow(AlignCenter)
	row.Add("yes", NewButton("Yes, Quit", &sdl.FRect{0, 0, 128, 32}, nil, false, func() { quit = true }))
	row.Add("no", NewButton("No", &sdl.FRect{0, 0, 128, 32}, nil, false, func() { confirmQuit.Close() }))

	confirmQuit.Recreate(root.IdealSize().X+32, root.IdealSize().Y+32)

	// // Confirm Load Menu - do this after Project.Modified works again.

	// confirmQuit := globals.MenuSystem.Add(NewMenu(&sdl.FRect{0, 0, 32, 32}, true), "confirmquit", true)
	// confirmQuit.Draggable = true

	// root = confirmQuit.Pages["root"]
	// root.AddRow(AlignCenter).Add("label", NewLabel("Are you sure you\nwish to quit?", nil, false, AlignCenter))
	// root.AddRow(AlignCenter).Add("label", NewLabel("Any unsaved changes will be lost.", nil, false, AlignCenter))
	// row = root.AddRow(AlignCenter)
	// row.Add("yes", NewButton("Yes, Quit", &sdl.FRect{0, 0, 128, 32}, nil, false, func() { quit = true }))
	// row.Add("no", NewButton("No", &sdl.FRect{0, 0, 128, 32}, nil, false, func() { confirmQuit.Close() }))

	// confirmQuit.Recreate(root.IdealSize().X+32, root.IdealSize().Y)

	// Settings Menu

	settings := NewMenu(&sdl.FRect{0, 0, 800, 512}, true)
	settings.CloseButtonEnabled = true
	settings.Draggable = true
	settings.Resizeable = true
	globals.MenuSystem.Add(settings, "settings", true)

	root = settings.Pages["root"]
	row = root.AddRow(AlignCenter)
	row.Add("header", NewLabel("Settings", nil, false, AlignCenter))

	row = root.AddRow(AlignCenter)
	row.Add("visual options", NewButton("Visual Options", nil, nil, false, func() {
		settings.SetPage("visual")
	}))

	row = root.AddRow(AlignCenter)
	row.Add("sound options", NewButton("Sound Options", nil, nil, false, func() {
		settings.SetPage("sound")
	}))

	row = root.AddRow(AlignCenter)
	row.Add("input", NewButton("Input", nil, nil, false, func() {
		settings.SetPage("input")
		settings.Pages["input"].FindElement("search editable", false).(*Label).SetText([]rune(""))
	}))

	row = root.AddRow(AlignCenter)
	row.Add("about", NewButton("About", nil, nil, false, func() {
		settings.SetPage("about")
	}))

	// Sound options

	sound := settings.AddPage("sound")
	sound.DefaultExpand = true

	row = sound.AddRow(AlignCenter)
	row.Add("", NewLabel("Sound Settings", nil, false, AlignCenter))

	row = sound.AddRow(AlignCenter)
	row.Add("", NewLabel("Timers Play Alarm Sound:", nil, false, AlignCenter))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsPlayAlarmSound)))

	row = sound.AddRow(AlignCenter)
	row.Add("", NewLabel("Audio Volume:", nil, false, AlignCenter))
	number := NewNumberSpinner(&sdl.FRect{0, 0, 256, 32}, false, globals.Settings.Get(SettingsAudioVolume))
	number.OnChange = func() {
		globals.Project.SendMessage(NewMessage(MessageVolumeChange, nil, nil))
	}
	row.Add("", number)

	// Visual options

	visual := settings.AddPage("visual")

	visual.OnUpdate = func() {
		// Refresh themes
		loadThemes()
		refreshThemes()
	}

	visual.DefaultExpand = true

	row = visual.AddRow(AlignCenter)
	row.Add("header", NewLabel("Visual Settings", nil, false, AlignCenter))

	row = visual.AddRow(AlignCenter)
	row.Add("theme label", NewLabel("Color Theme:", nil, false, AlignLeft))

	drop := NewDropdown(&sdl.FRect{0, 0, 128, 32}, false, func(index int) {
		globals.Settings.Get(SettingsTheme).Set(availableThemes[index])
		refreshThemes()
	}, availableThemes...)

	drop.OnOpen = func() {
		loadThemes()
		drop.SetOptions(availableThemes...)
	}

	for i, k := range availableThemes {
		if globals.Settings.Get(SettingsTheme).AsString() == k {
			drop.ChosenIndex = i
			break
		}
	}

	row.Add("theme dropdown", drop)

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Always Show Numbering:", nil, false, AlignLeft))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsAlwaysShowNumbering)))

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Show Status Messages:", nil, false, AlignLeft))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsDisplayMessages)))

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Focused FPS:", nil, false, AlignLeft))
	num := NewNumberSpinner(nil, false, globals.Settings.Get(SettingsTargetFPS))
	num.MinValue = 5
	row.Add("", num)

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Unfocused FPS:", nil, false, AlignLeft))
	num = NewNumberSpinner(nil, false, globals.Settings.Get(SettingsUnfocusedFPS))
	num.MinValue = 5
	row.Add("", num)

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Show Grid:", nil, false, AlignLeft))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsShowGrid)))

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Save Window Position:", nil, false, AlignLeft))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsSaveWindowPosition)))

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Flash Selected Cards:", nil, false, AlignLeft))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsFlashSelected)))

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Focus on Elapsed Timers:", nil, false, AlignLeft))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsFocusOnElapsedTimers)))

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Notify on Elapsed Timers:", nil, false, AlignLeft))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsNotifyOnElapsedTimers)))

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Show About Dialog On Start:", nil, false, AlignLeft))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsShowAboutDialogOnStart)))

	row = visual.AddRow(AlignCenter)
	row.Add("", NewSpacer(nil))

	row = visual.AddRow(AlignCenter)
	row.Add("", NewLabel("Custom Font Path:", nil, false, AlignLeft))
	fontPath := NewLabel("Font path", nil, false, AlignLeft)
	fontPath.Editable = true
	fontPath.Property = globals.Settings.Get(SettingsCustomFontPath)
	fontPath.OnClickOut = func() {
		globals.TriggerReloadFonts = true
	}
	row.Add("", fontPath)

	row = visual.AddRow(AlignCenter)
	row.Add("", NewButton("Browse", nil, nil, false, func() {

		if path, err := zenity.SelectFile(zenity.Title("Select Custom Font (.ttf, .otf)"), zenity.FileFilter{Name: "Font Files", Patterns: []string{"*.ttf", "*.otf"}}); err == nil {
			globals.Settings.Get(SettingsCustomFontPath).Set(path)
			globals.TriggerReloadFonts = true
		}

	}))

	row.Add("", NewButton("Clear", nil, nil, false, func() {
		globals.Settings.Get(SettingsCustomFontPath).Set("")
		globals.TriggerReloadFonts = true
	}))
	// row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsShowAboutDialogOnStart)))

	// INPUT PAGE

	var rebindingKey *Button
	var rebindingShortcut *Shortcut
	heldKeys := []sdl.Keycode{}
	heldButtons := []uint8{}

	input := settings.AddPage("input")
	input.OnUpdate = func() {

		globals.Keybindings.On = false

		if rebindingKey != nil {

			rebindingKey.Label.SetText([]rune("Rebinding..."))

			if globals.Keyboard.Key(sdl.K_ESCAPE).Pressed() {
				rebindingKey = nil
				rebindingShortcut = nil
			} else {

				if (len(globals.Keyboard.HeldKeys()) == 0 && len(heldKeys) > 0) || (len(globals.Mouse.HeldButtons()) == 0 && len(heldButtons) > 0) {

					if len(heldButtons) > 0 {
						rebindingShortcut.SetButton(heldButtons[0], heldKeys...)
					} else if len(heldKeys) > 0 {
						rebindingShortcut.SetKeys(heldKeys[len(heldKeys)-1], heldKeys[:len(heldKeys)-1]...)
					}
					globals.Keybindings.UpdateShortcutFamilies()

					rebindingKey = nil
					rebindingShortcut = nil
					heldKeys = []sdl.Keycode{}
					heldButtons = []uint8{}

					SaveSettings()

				} else {

					if pressed := globals.Keyboard.PressedKeys(); len(pressed) > 0 {

						added := false
						for _, h := range heldKeys {
							if h == pressed[0] {
								added = true
							}
						}
						if !added {
							heldKeys = append(heldKeys, pressed[0])
						}

					} else if pressed := globals.Mouse.PressedButtons(); len(pressed) > 0 {
						heldButtons = append(heldButtons, pressed[0])
					}

				}

			}

		} else {

			for name, shortcut := range globals.Keybindings.Shortcuts {
				b := input.FindElement(name+"-b", false).(*Button)
				b.Label.SetText([]rune(shortcut.KeysToString()))

				d := input.FindElement(name+"-d", false).(*Button)
				d.Disabled = shortcut.IsDefault()
				if d.Disabled {
					d.Label.SetText([]rune("---"))
				} else {
					d.Label.SetText([]rune("Reset To Default"))
				}
			}

		}

	}

	row = input.AddRow(AlignCenter)
	row.Add("input header", NewLabel("-Input-", nil, false, AlignLeft))

	row = input.AddRow(AlignCenter)
	row.Add("", NewLabel("Double-click: ", nil, false, AlignLeft))
	dropdown := NewDropdown(nil, false, nil, DoubleClickLast, DoubleClickCheckbox, DoubleClickNothing)
	dropdown.Property = globals.Settings.Get(SettingsDoubleClickMode)
	row.Add("", dropdown)

	row = input.AddRow(AlignCenter)
	row.Add("", NewLabel("Reverse panning direction: ", nil, false, AlignLeft))
	row.Add("", NewCheckbox(0, 0, false, globals.Settings.Get(SettingsReversePan)))

	row = input.AddRow(AlignCenter)
	row.Add("keybindings header", NewLabel("Keybindings", nil, false, AlignLeft))

	row = input.AddRow(AlignCenter)
	row.Add("search label", NewLabel("Search: ", nil, false, AlignLeft))
	searchKeybindingsLabel := NewLabel("test", &sdl.FRect{0, 0, 380, 32}, false, AlignLeft)
	searchKeybindingsLabel.Editable = true
	// searchKeybindingsLabel.AutoExpand = true
	searchKeybindingsLabel.OnChange = func() {

		text := strings.TrimSpace(searchKeybindingsLabel.TextAsString())
		for _, row := range input.FindRows("key-", true) {
			if text == "" {
				row.Visible = true
			} else {
				if row.FindElement(text, true) != nil {
					row.Visible = true
				} else {
					row.Visible = false
				}
			}
		}

	}
	row.Add("search editable", searchKeybindingsLabel)
	row.Add("clear button", NewIconButton(0, 0, &sdl.Rect{176, 0, 32, 32}, false, func() {
		searchKeybindingsLabel.SetText([]rune(""))
	}))
	// row.ExpandElements = true

	row = input.AddRow(AlignCenter)
	row.Add("reset all to default", NewButton("Reset All Bindings to Default", nil, nil, false, func() {
		for _, shortcut := range globals.Keybindings.Shortcuts {
			shortcut.ResetToDefault()
			globals.Keybindings.UpdateShortcutFamilies()
		}
	}))

	for _, s := range globals.Keybindings.ShortcutsInOrder {

		// Make a copy so the OnPressed() function below refers to "this" shortcut, rather than the last one in the range
		shortcut := s

		row = input.AddRow(AlignCenter)

		row.Add("key-"+shortcut.Name, NewLabel(shortcut.Name, nil, false, AlignLeft))
		row.ExpandElements = true

		redefineButton := NewButton(shortcut.KeysToString(), nil, nil, false, nil)

		redefineButton.OnPressed = func() {
			rebindingKey = redefineButton
			rebindingShortcut = shortcut
		}

		row.Add(shortcut.Name+"-b", redefineButton)

		button := NewButton("Reset to Default", nil, nil, false, nil)

		button.OnPressed = func() {
			shortcut.ResetToDefault()
			globals.Keybindings.UpdateShortcutFamilies()
		}

		row.Add(shortcut.Name+"-d", button)
	}

	about := settings.AddPage("about")

	about.DefaultExpand = true

	row = about.AddRow(AlignCenter)
	row.Add("", NewLabel("About", nil, false, AlignCenter))
	row = about.AddRow(AlignCenter)
	row.Add("", NewLabel("Welcome to MasterPlan!", nil, false, AlignCenter))
	row = about.AddRow(AlignCenter)
	row.Add("", NewSpacer(nil))

	row = about.AddRow(AlignCenter)
	row.Add("", NewLabel("This is an alpha of the next update, v0.8.0. As this is just an alpha, it hasn't reached feature parity with the previous version (v0.7) just yet.", &sdl.FRect{0, 0, 512, 96}, false, AlignLeft))

	row = about.AddRow(AlignCenter)
	row.Add("", NewSpacer(nil))

	row = about.AddRow(AlignCenter)
	row.Add("", NewLabel("That said, I think this is already FAR better than v0.7 and am very excited to get people using it and get some feedback on the new changes. Please do let me know your thoughts! (And don't forget to do frequent back-ups!) ~ SolarLune", &sdl.FRect{0, 0, 512, 160}, false, AlignLeft))

	row = about.AddRow(AlignCenter)
	row.Add("", NewSpacer(nil))

	row = about.AddRow(AlignCenter)
	row.ExpandElements = false
	row.Add("", NewButton("Discord", nil, &sdl.Rect{48, 224, 32, 32}, false, func() { browser.OpenURL("https://discord.gg/tRVf7qd") }))
	row.Add("", NewSpacer(nil))
	row.Add("", NewButton("Overview Devlog", nil, nil, false, func() { browser.OpenURL("https://youtu.be/43sotReXnGA") }))
	row.Add("", NewSpacer(nil))
	row.Add("", NewButton("Twitter", nil, &sdl.Rect{80, 224, 32, 32}, false, func() { browser.OpenURL("https://twitter.com/MasterPlanApp") }))

	// Search Menu

	search := globals.MenuSystem.Add(NewMenu(&sdl.FRect{0, 0, 512, 96}, true), "search", false)
	search.Center()
	search.CloseButtonEnabled = true
	search.Draggable = true
	search.Resizeable = true

	root = search.Pages["root"]
	row = root.AddRow(AlignCenter)
	row.Add("", NewLabel("Find:", nil, false, AlignCenter))
	searchLabel := NewLabel("Text", &sdl.FRect{0, 0, 256, 32}, false, AlignLeft)
	searchLabel.Editable = true
	searchLabel.RegexString = RegexNoNewlines()

	foundLabel := NewLabel("0 of 0", &sdl.FRect{0, 0, 128, 32}, false, AlignCenter)
	foundCards := []*Card{}
	foundIndex := 0

	caseSensitive := false

	findFunc := func() {

		page := globals.Project.CurrentPage
		page.Selection.Clear()
		foundCards = []*Card{}

		if len(searchLabel.Text) == 0 {
			foundLabel.SetText([]rune("0 of 0"))
			return
		}

		for _, card := range page.Cards {

			for propName, prop := range card.Properties.Props {

				if (propName == "description" || propName == "filepath") && prop.InUse && prop.IsString() {

					propString := prop.AsString()
					searchString := searchLabel.TextAsString()

					if !caseSensitive {
						propString = strings.ToLower(prop.AsString())
						searchString = strings.ToLower(searchString)
					}

					if strings.Contains(propString, searchString) {
						foundCards = append(foundCards, card)
						continue
					}
				}

			}

		}

		if foundIndex >= len(foundCards) {
			foundIndex = 0
		} else if foundIndex < 0 {
			foundIndex = len(foundCards) - 1
		}

		if len(foundCards) > 0 {
			page.Selection.Add(foundCards[foundIndex])
			foundLabel.SetText([]rune(fmt.Sprintf("%d of %d", foundIndex+1, len(foundCards))))
			globals.Project.Camera.FocusOn(foundCards[foundIndex])
		} else {
			foundLabel.SetText([]rune("0 of 0"))
		}

	}

	searchLabel.OnChange = func() {
		foundIndex = 0
		findFunc()
	}

	root.OnUpdate = func() {

		if globals.Keybindings.Pressed(KBFindNext) {
			foundIndex++
			findFunc()
		} else if globals.Keybindings.Pressed(KBFindPrev) {
			foundIndex--
			findFunc()
		}

	}

	search.OnOpen = func() {
		searchLabel.Editing = true
		searchLabel.Selection.SelectAll()
	}

	var caseSensitiveButton *IconButton
	caseSensitiveButton = NewIconButton(0, 0, &sdl.Rect{112, 224, 32, 32}, false, func() {
		caseSensitive = !caseSensitive
		if caseSensitive {
			caseSensitiveButton.IconSrc.X = 144
		} else {
			caseSensitiveButton.IconSrc.X = 112
		}
		findFunc()
	})
	row.Add("", caseSensitiveButton)

	row.Add("", NewIconButton(0, 0, &sdl.Rect{176, 96, 32, 32}, false, func() {
		searchLabel.SetText([]rune(""))
	}))

	row.Add("", searchLabel)

	row = root.AddRow(AlignCenter)

	prev := NewIconButton(0, 0, &sdl.Rect{112, 32, 32, 32}, false, func() {
		foundIndex--
		findFunc()
	})
	prev.Flip = sdl.FLIP_HORIZONTAL
	row.Add("", prev)

	row.Add("", foundLabel)

	row.Add("", NewIconButton(0, 0, &sdl.Rect{112, 32, 32, 32}, false, func() {
		foundIndex++
		findFunc()
	}))

	// Stats Menu

	stats := globals.MenuSystem.Add(NewMenu(&sdl.FRect{0, 0, 700, 128}, true), "stats", false)
	stats.Center()
	stats.Draggable = true
	stats.CloseButtonEnabled = true
	stats.Resizeable = true

	root = stats.Pages["root"]

	row = root.AddRow(AlignCenter)
	row.Add("", NewLabel("-Stats-", nil, false, AlignCenter))

	row = root.AddRow(AlignLeft)
	maxLabel := NewLabel("so many cards existing", nil, false, AlignLeft)
	row.Add("", maxLabel)

	row = root.AddRow(AlignLeft)
	completedLabel := NewLabel("so many cards completed", nil, false, AlignLeft)
	row.Add("", completedLabel)
	row.ExpandElements = true

	row = root.AddRow(AlignLeft)
	row.Add("", NewSpacer(&sdl.FRect{0, 0, 32, 1}))

	row = root.AddRow(AlignLeft)
	row.Add("time estimation", NewLabel("Time Estimation:", nil, false, AlignLeft))

	timeNumber := NewLabel("15", &sdl.FRect{0, 0, 128, 32}, false, AlignCenter)
	timeNumber.Editable = true
	timeNumber.RegexString = RegexOnlyDigits()
	row.Add("", timeNumber)

	timeUnitChoices := []string{
		"Minutes",
		"Hours",
		"Days",
		"Weeks",
		"Months",
	}

	row = root.AddRow(AlignLeft)
	row.ExpandElements = true
	timeUnit := NewButtonGroup(&sdl.FRect{0, 0, 32, 32}, false, func(index int) {}, globals.Settings.Get("time unit"), timeUnitChoices...)

	// timeUnit := NewDropdown(nil, false, func(index int) {}, timeUnitChoices...)
	row.Add("", timeUnit)

	row = root.AddRow(AlignLeft)
	estimatedTime := NewLabel("Time estimation label", nil, false, AlignLeft)
	row.Add("", estimatedTime)
	row.ExpandElements = true

	row = root.AddRow(AlignLeft)
	row.Add("", NewLabel("Limit time estimation read-out to same as units: ", nil, false, AlignLeft))

	limitTimeCheckbox := NewCheckbox(0, 0, false, nil)
	row.Add("", limitTimeCheckbox)

	root.OnUpdate = func() {

		maxLabel.SetText([]rune(fmt.Sprintf("Total Cards: %d Cards", len(globals.Project.CurrentPage.Cards))))

		completionLevel := float32(0)
		maxLevel := float32(0)
		totalCompletable := 0
		completedCards := 0

		for _, i := range globals.Project.CurrentPage.Cards {

			if i.Numberable() {

				maxLevel += i.MaximumCompletionLevel()
				completionLevel += i.CompletionLevel()

				totalCompletable++
				if i.Completed() {
					completedCards++
				}

			}

		}

		if maxLevel == 0 {
			completedLabel.SetText([]rune("Total Cards Completed: 0 / 0 (0%)"))
		} else {
			completedLabel.SetText([]rune(fmt.Sprintf("Total Cards Completed: %d / %d (%d%%)", int(completedCards), int(totalCompletable), int(float32(completedCards)/float32(totalCompletable)*100))))
		}

		if completionLevel < maxLevel {
			var unit time.Duration
			t := timeUnitChoices[timeUnit.ChosenIndex]
			switch t {
			case "Minutes":
				unit = time.Minute
			case "Hours":
				unit = time.Minute * 60
			case "Days":
				unit = time.Minute * 60 * 24
			case "Weeks":
				unit = time.Minute * 60 * 24 * 7
			case "Months":
				unit = time.Minute * 60 * 24 * 30
			}
			duration := unit * time.Duration(float32(timeNumber.TextAsInt())*(maxLevel-completionLevel)*10) / 10
			s := durafmt.Parse(duration)
			if limitTimeCheckbox.Checked {

				// durafmt has no concept of "months"
				if t == "Months" {
					t = "Weeks"
				}

				s = s.LimitToUnit(strings.ToLower(t))
			} else {
				s = s.LimitFirstN(2)
			}
			estimatedTime.SetText([]rune(fmt.Sprintf("%s to completion.", s)))
		} else {
			estimatedTime.SetText([]rune(fmt.Sprintf("All tasks completed.")))
		}

	}

}

func profileCPU() {

	// rInt, _ := rand.Int(rand.Reader, big.NewInt(400))
	// cpuProfFile, err := os.Create(fmt.Sprintf("cpu.pprof%d", rInt))
	cpuProfFile, err := os.Create("cpu.pprof")
	if err != nil {
		log.Fatal("Could not create CPU Profile: ", err)
	}
	pprof.StartCPUProfile(cpuProfFile)
	globals.EventLog.Log("CPU Profiling begun.")

	time.AfterFunc(time.Second*2, func() {
		cpuProfileStart = time.Time{}
		pprof.StopCPUProfile()
	})

}
