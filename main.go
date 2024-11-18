package main

//
// - [ ] Pallette of useful character sets, e.g. for box drawing, click to select
// - [ ] Primary/secondary brush, left/right mouse button, swap with some hotkey
// - [ ] Brush size
// - [x] Enter text commands, like : in vim
// - [x] Save-load functionality
// - [ ] Coloring
// - [ ] Undo and redo
// - [ ] Draw a border around the canvas
// - [ ] Layers and transparency
// - [ ] Move layers around
// - [ ] On-screen ruler
//

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type pixel rune

func newCanvas(width, height int) [][]pixel {
	c := make([][]pixel, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c[y] = append(c[y], ' ')
		}
	}
	return c
}

func newTestCanvas(width, height int) [][]pixel {
	c := make([][]pixel, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if (x + y) % 2 == 0 {
				c[y] = append(c[y], ' ')
			} else {
				c[y] = append(c[y], '#')
			}
		}
	}
	return c
}

type model struct {
	width int
	height int
	canvas [][]pixel
	brush pixel

	commandBuffer string
	commandActive bool
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.EnableMouseAllMotion, tea.ClearScreen)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	log.Printf("msg: %q %T", msg, msg)
	switch msg := msg.(type) {
	case quitMsg:
		return m, tea.Quit
	case canvasLoadedMsg:
		m.width = msg.width
		m.height = msg.height
		m.canvas = msg.canvas
		return m, nil
	case brushChangedMsg:
		m.brush = msg.brush
		return m, nil
	case tea.MouseMsg:
		log.Printf("msg action=%q button=%q", msg.Action, msg.Button)
		switch msg.Action {
		case tea.MouseActionPress:
			log.Printf("X=%d Y=%d", msg.X, msg.Y)
			if msg.X < m.width && msg.Y < m.height {
				m.canvas[msg.Y][msg.X] = m.brush
				return m, nil
			}
		case tea.MouseActionMotion:
			log.Printf("X=%d Y=%d", msg.X, msg.Y)
			if msg.Button == tea.MouseButtonLeft && msg.X < m.width && msg.Y < m.height {
				m.canvas[msg.Y][msg.X] = m.brush
				return m, nil
			}
		}
	case tea.KeyMsg:
		if m.commandActive && msg.String() == "enter" {
			cmd := m.commandBuffer
			m.commandBuffer = ""
			m.commandActive = false
			return m, interpretCmd(m, cmd)
		} else if m.commandActive && msg.String() == "backspace" {
			m.commandBuffer = m.commandBuffer[:len(m.commandBuffer)-1]
			return m, nil
		} else if m.commandActive && msg.String() == "ctrl+c" {
			m.commandActive = false
			return m, nil
		} else if m.commandActive {
			m.commandBuffer += msg.String()
			return m, nil
		}
		switch msg.String() {

		case ":":
			m.commandActive = true
			return m, nil

		case "ctrl+c", "q":
			return m, tea.Quit

		default:
			m.brush = pixel(msg.String()[0])
			return m, nil
		}
	}
	return m, nil
}

func (m model) View() string {
	var buffer bytes.Buffer

	if err := dumpCanvas(m.canvas, m.width, m.height, &buffer); err != nil {
		log.Printf("err: %q", err)
	}
	if m.commandActive {
		fmt.Fprintf(&buffer, ":%s█\n", m.commandBuffer)
	}
	return buffer.String()
}

type quitMsg struct {}

type canvasLoadedMsg struct {
	width int
	height int
	canvas [][]pixel
}

type brushChangedMsg struct {
	brush pixel
}

func interpretCmd(m model, command string) tea.Cmd {
	return func() tea.Msg {
		if command == "q" || command == "quit" {
			return quitMsg{}
		}

		split := strings.SplitN(command, " ", 2)
		verb := split[0]
		rest := split[1]

		switch verb {
		case "q", "quit":
			return quitMsg{}

		case "s", "save":
			fout, err := os.OpenFile(rest, os.O_WRONLY | os.O_CREATE, 0o644)
			if err != nil {
				log.Printf("err: %q", err)
				return nil
			}
			fmt.Fprintf(fout, "%d %d\n", m.width, m.height)
			defer fout.Close()

			if err := dumpCanvas(m.canvas, m.width, m.height, fout); err != nil {
				log.Printf("err: %q", err)
				return nil
			}

		case "l", "load":
			fin, err := os.Open(rest)
			if err != nil {
				log.Printf("err: %q", err)
				return nil
			}
			defer fin.Close()

			width, height, canvas, err := loadCanvas(fin)
			if err != nil {
				log.Printf("err: %q", err)
				return nil
			}

			return canvasLoadedMsg{width, height, canvas}

		case "b", "brush":
			rest = strings.ToLower(rest)
			if strings.HasPrefix(rest, "\\u") || strings.HasPrefix(rest, "u+") {
				if codePoint, err := strconv.ParseInt(rest[2:], 16, 64); err == nil {
					return brushChangedMsg{pixel(codePoint)}
				}
			}
			return brushChangedMsg{pixel(rest[0])}
		}
		return nil
	}
}

func loadCanvas(fin io.Reader) (width, height int, canvas [][]pixel, err error) {
	reader := bufio.NewReader(fin)
	firstLine, err := reader.ReadBytes('\n')
	if err != nil {
		return 0, 0, nil, err
	}
	split := strings.SplitN(strings.TrimRight(string(firstLine), " \n"), " ", 2)
	if width, err = strconv.Atoi(split[0]); err != nil {
		return 0, 0, nil, err
	}
	if height, err = strconv.Atoi(split[1]); err != nil {
		return 0, 0, nil, err
	}

	canvas = make([][]pixel, height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, _, err := reader.ReadRune()
			if err != nil {
				return 0, 0, nil, err
			}
			canvas[y] = append(canvas[y], pixel(r))
		}
		//
		// Read EOL
		//
		if _, err := reader.ReadBytes('\n'); err != nil {
			return 0, 0, nil, err
		}
	}

	return width, height, canvas, nil
}

func dumpCanvas(canvas [][]pixel, width, height int, fout io.Writer) error {
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if _, err := fmt.Fprintf(fout, string(canvas[y][x])); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(fout, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	logpath := filepath.Join(os.TempDir(), "gopnik.log")
	log.Printf("redirecting stderr to %s", logpath)
	f, err := tea.LogToFile(logpath, "debug")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	m := model{
		width: 80,
		height: 50,
		canvas: newCanvas(80, 50),
		brush: '#',
	}

	program := tea.NewProgram(m)
	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}
}
