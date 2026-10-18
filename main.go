package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
	"uuid"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const jokeAPI = "https://icanhazdadjoke.com/"
const cooldown = 2 * time.Second
const httpTimeout = 5 * time.Second
const waitDisplay = 2 * time.Second

type jokeMsg struct {
	joke string
}

type jokeErrMsg struct {
	err error
}

type jokeRes struct {
	Joke string `json:"joke"`
}

type clearWaitMsg struct {
	waitID uuid.UUID
}

type model struct {
	spinner   spinner.Model
	joke      string
	err       error
	loading   bool
	width     int
	wait      bool
	waitID    uuid.UUID
	helpGuide string
	lastFetch time.Time
}

type binding struct {
	keys        []string
	description string
}

var (
	quit        = binding{keys: []string{"ctrl+c", "q", "esc"}, description: "quit"}
	refresh     = binding{keys: []string{"r", "space"}, description: "fetch joke"}
	allBindings = []binding{refresh, quit}
)

var client = &http.Client{Timeout: httpTimeout}

func buildHelpGuide() string {
	guideFormat := "%s: %s\n"
	var sb strings.Builder
	for _, elem := range allBindings {
		fmt.Fprintf(&sb, guideFormat, elem.description, strings.Join(elem.keys, ","))
	}
	return fmt.Sprintf("--Commands--\n%s", strings.TrimSuffix(sb.String(), "\n"))
}

func initialModel() model {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	return model{spinner: spin, loading: true, helpGuide: buildHelpGuide()}
}

func fetchJoke() tea.Cmd {
	return func() tea.Msg {
		req, err := http.NewRequest(http.MethodGet, jokeAPI, nil)
		if err != nil {
			return jokeErrMsg{err: fmt.Errorf("unable to run request: %w", err)}
		}

		req.Header.Add("Accept", "application/json")
		// Joke api requests a custom user agent is added to help identify traffic
		req.Header.Add("User-Agent", "terminal-api-user/1.0")

		res, err := client.Do(req)

		if err != nil {
			return jokeErrMsg{err: fmt.Errorf("unable to get joke: %w", err)}
		}

		defer res.Body.Close()

		if res.StatusCode >= 300 {
			return jokeErrMsg{err: fmt.Errorf("joke api returned status code: %d", res.StatusCode)}
		}

		var body jokeRes
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			return jokeErrMsg{err: fmt.Errorf("could not parse joke response: %w", err)}
		}
		return jokeMsg{joke: body.Joke}
	}
}

func startTimer(duration time.Duration, waitID uuid.UUID) tea.Cmd {
	return tea.Tick(duration, func(t time.Time) tea.Msg {
		return clearWaitMsg{waitID: waitID}
	})
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchJoke())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case slices.Contains(quit.keys, msg.String()):
			return m, tea.Quit
		case slices.Contains(refresh.keys, msg.String()):
			if m.loading {
				return m, nil
			}
			if time.Since(m.lastFetch) < cooldown {
				if m.wait {
					return m, nil
				}
				m.wait = true
				m.waitID = uuid.New()
				return m, startTimer(waitDisplay, m.waitID)
			}
			m.loading = true
			m.err = nil
			m.wait = false
			return m, tea.Batch(m.spinner.Tick, fetchJoke())
		}

	case jokeMsg:
		m.lastFetch = time.Now()
		m.loading = false
		m.joke = msg.joke

	case jokeErrMsg:
		m.loading = false
		m.err = msg.err

	case clearWaitMsg:
		if msg.waitID != m.waitID {
			return m, nil
		}
		m.wait = false

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		newSpinner, cmd := m.spinner.Update(msg)
		m.spinner = newSpinner
		return m, cmd
	}

	return m, nil
}

func (m model) View() tea.View {
	// Styling to limit text on a line to the width of the screen
	style := lipgloss.NewStyle().Width(m.width)

	var frontMsg string
	if m.wait {
		frontMsg = "Wait!"
	} else if m.loading {
		frontMsg = m.spinner.View()
	} else if m.err != nil {
		frontMsg = m.err.Error()
	} else {
		frontMsg = m.joke
	}

	s := fmt.Sprintf("%s\n\n%s", frontMsg, m.helpGuide)

	view := tea.NewView(style.Render(s))
	view.AltScreen = true
	return view
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Println("Unable to run program:", err)
		os.Exit(1)
	}
}
