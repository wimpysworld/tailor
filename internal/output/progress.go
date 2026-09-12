package output

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/wimpysworld/tailor/internal/termtext"
)

type (
	activateSpinner struct {
		id         string
		generation uint64
	}
	stopProgress   struct{}
	progressConfig struct {
		animate, verbose, colour, ascii bool
	}
)

// ProgressModel is the testable stage lifecycle used by the inline programme.
type ProgressModel struct {
	spinner       spinner.Model
	stage         StageEvent
	generation    uint64
	active        bool
	colour, ascii bool
}

func NewProgressModel() ProgressModel { return newProgressModel(false, false) }
func newProgressModel(colour, ascii bool) ProgressModel {
	model := ProgressModel{spinner: spinner.New(), colour: colour, ascii: ascii}
	model.spinner.Spinner = spinner.MiniDot
	if ascii {
		model.spinner.Spinner = spinner.Line
	}
	return model
}

func (m ProgressModel) Init() tea.Cmd {
	if m.active {
		return m.spinner.Tick
	}
	return nil
}

func (m ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case StageEvent:
		m.stage = msg
		if msg.Phase == "complete" || msg.Phase == "error" {
			m.active = false
			return m, nil
		}
		m.active = false
		m.generation++
		id, generation := msg.ID, m.generation
		return m, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
			return activateSpinner{id: id, generation: generation}
		})
	case activateSpinner:
		if m.stage.ID == msg.id && m.generation == msg.generation && m.stage.Phase == "start" {
			m.active = true
			return m, m.spinner.Tick
		}
	case spinner.TickMsg:
		if m.active {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case stopProgress:
		return m, tea.Quit
	}
	return m, nil
}

func (m ProgressModel) View() tea.View {
	prefix := "*"
	colour := "6"
	if m.active {
		prefix = m.spinner.View()
	}
	if m.stage.Phase == "complete" {
		prefix, colour = "ok", "2"
	}
	if m.stage.Phase == "error" {
		prefix, colour = "!", "1"
	}
	label := termtext.EscapeControlText(m.stage.Label)
	if label == "" {
		return tea.NewView("")
	}
	if m.colour {
		prefix = lipgloss.NewStyle().Foreground(lipgloss.Color(colour)).Bold(true).Render(prefix)
	}
	line := prefix + "  " + label
	details := make([]string, 0, 3)
	if m.stage.Total > 0 {
		details = append(details, fmt.Sprintf("%d of %d", m.stage.Current, m.stage.Total))
	}
	if m.stage.Item != "" {
		details = append(details, termtext.EscapeControlText(m.stage.Item))
	}
	if m.stage.Err != nil {
		details = append(details, termtext.EscapeControlText(m.stage.Err.Error()))
	}
	if len(details) > 0 {
		line += "\n   " + strings.Join(details, " · ")
	}
	return tea.NewView(line)
}

// Progress runs one Bubble Tea programme in inline mode on stderr.
type Progress struct {
	program    *tea.Program
	writer     io.Writer
	config     progressConfig
	timer      *time.Timer
	generation uint64
	stage      StageEvent
	stopped    bool
	done       chan struct{}
	once       sync.Once
	mu         sync.Mutex
}

// StartProgress starts a colourless Unicode live display for compatibility.
// Command code starts progress through Policy.StartProgress instead.
func StartProgress(stderr io.Writer) *Progress {
	return startProgress(stderr, progressConfig{animate: true})
}

func startProgress(stderr io.Writer, config progressConfig) *Progress {
	if stderr == nil {
		stderr = io.Discard
	}
	return &Progress{writer: stderr, config: config, done: make(chan struct{})}
}

func (p *Progress) Observe(event StageEvent) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	if p.config.verbose {
		fmt.Fprintln(p.writer, stageRecord(event))
		return
	}
	if p.program != nil {
		p.program.Send(event)
		return
	}
	p.stage = event
	p.generation++
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	if p.config.animate && event.Phase == "start" {
		// Bubble Tea probes terminal modes even with input disabled. Do not
		// start its terminal lifecycle for a stage that finishes before the delay.
		generation := p.generation
		p.timer = time.AfterFunc(300*time.Millisecond, func() { p.activate(generation) })
	}
}

func (p *Progress) activate(generation uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.program != nil || p.generation != generation {
		return
	}
	model := newProgressModel(p.config.colour, p.config.ascii)
	model.stage, model.active = p.stage, true
	p.program = tea.NewProgram(model, tea.WithInput(nil), tea.WithOutput(p.writer), tea.WithoutSignalHandler())
	go func() { _, _ = p.program.Run(); close(p.done) }()
}

func stageRecord(event StageEvent) string {
	parts := []string{"[" + termtext.EscapeControlText(event.Phase) + "]", termtext.EscapeControlText(event.Label)}
	if event.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d of %d", event.Current, event.Total))
	}
	if event.Item != "" {
		parts = append(parts, termtext.EscapeControlText(event.Item))
	}
	if event.Err != nil {
		parts = append(parts, termtext.EscapeControlText(event.Err.Error()))
	}
	return strings.Join(parts, " · ")
}

// WarningWriter prints above the live display, then Bubble Tea redraws the active stage.
func (p *Progress) WarningWriter(fallback io.Writer) io.Writer {
	if p == nil {
		return fallback
	}
	return writerFunc(func(data []byte) (int, error) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.program == nil || p.stopped {
			return fallback.Write(data)
		}
		p.program.Printf("%s", data)
		return len(data), nil
	})
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(data []byte) (int, error) { return f(data) }
func (p *Progress) Stop() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.stopped = true
		if p.timer != nil {
			p.timer.Stop()
		}
		if p.program != nil {
			p.program.Send(stopProgress{})
			<-p.done
		}
	})
}
