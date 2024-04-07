package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joerdav/xc/models"
	"github.com/joerdav/xc/run"
)

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(titleMargin)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(itemPadding)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(selectedItemPadding).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(paginationPadding)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(helpPadding).PaddingBottom(1)
)

const (
	titleMargin         = 2
	itemPadding         = 4
	selectedItemPadding = 2
	paginationPadding   = 4
	helpPadding         = 4
	listItemWidth       = 20
	listItemHeight      = 6
)

type taskItem struct {
	models.Task
}

func (ti taskItem) FilterValue() string {
	return ti.Name
}

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(taskItem)
	if !ok {
		return
	}

	str := i.Name

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type model struct {
	list     list.Model
	choice   *models.Task
	quitting bool
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			i, ok := m.list.SelectedItem().(taskItem)
			if ok {
				m.choice = &i.Task
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	return "\n" + m.list.View()
}

func interactivePicker(ctx context.Context, tasks []models.Task, dir string) error {
	var items []list.Item
	for _, t := range tasks {
		items = append(items, taskItem{t})
	}
	l := list.New(items, itemDelegate{}, listItemWidth, listItemHeight+len(tasks))
	l.Title = "xc: Choose a task"
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings()
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle

	m := model{list: l}
	tm, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}
	task := tm.(model).choice
	if task == nil {
		return nil
	}
	runner, err := run.NewRunner(tasks, dir)
	if err != nil {
		return fmt.Errorf("xc parse error: %w", err)
	}
	err = runner.Run(ctx, task.Name, nil)
	fmt.Println("Task name: " + task.Name)
	if err != nil {
		return fmt.Errorf("xc: %w", err)
	}
	err = addToShellHistory("xc " + task.Name)
	if err != nil {
		return fmt.Errorf("xc: %w", err)
	}
	return nil
}

// Function to detect shell and append command to history with proper handling for Zsh and Bash
func addToShellHistory(command string) error {
	currentUser, err := user.Current()
	if err != nil {
		return err
	}
	homeDir := currentUser.HomeDir

	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "bash") {
		historyFile := filepath.Join(homeDir, ".bash_history")
		return appendToBashHistory(historyFile, command)
	} else if strings.Contains(shell, "zsh") {
		historyFile := filepath.Join(homeDir, ".zsh_history")
		return appendToZshHistory(historyFile, command)
	}
	return nil
}

// Bash-specific function for appending to history
func appendToBashHistory(historyFile, command string) error {
	return appendToHistoryFile(historyFile, command+"\n")
}

// Zsh-specific function for appending to history, including formatting with timestamp
func appendToZshHistory(historyFile, command string) error {
	timestamp := time.Now().Unix()
	formattedCommand := fmt.Sprintf(": %d:0;%s\n", timestamp, command)

	err := appendToHistoryFile(historyFile, formattedCommand)
	if err != nil {
		return err
	}
	return callRefreshScript()
}

// General function to append a command to a history file
func appendToHistoryFile(historyFile, command string) error {
	file, err := os.OpenFile(historyFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.WriteString(command); err != nil {
		return err
	}
	return nil
}

func callRefreshScript() error {
	scriptPath := "cmd/xc/refresh_history.zsh"

	cmd := exec.Command(scriptPath)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
