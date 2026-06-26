package main

import (
	// "encoding/json"
	// "charm.land/bubbles/v2/list"
	// "charm.land/bubbles/v2/textinput"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/PuerkitoBio/goquery"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

const (
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	Bold    = "\033[1m"
	Reset   = "\033[0m"
)

const listHeight = 10

type queryModel struct {
	Id         int
	title      string
	currency   string
	price      string
	priceFloat float64
	link       string
	converted  bool
}

type queryResultMsg []queryModel

type canvasT struct {
	width int
	flip  bool
}

const (
	INPUT    = 0
	SEARCHED = 1
)

type model struct {
	Panel      int
	Keyword    string
	spinner    spinner.Model
	canvas     canvasT
	QModel     []queryModel
	QSorted    []queryModel
	QModelCopy []queryModel
	sort       bool
	sortPrice  bool
	cursor     int // which to-do list item our cursor is pointing at
	startIndex int
	height     int
	selected   map[int]struct{} // which to-do items are selected
	viewport   viewport.Model
	input      textinput.Model
	err        error
}

func sortPrice(qm []queryModel) []queryModel {
	for i := range qm {
		if !qm[i].converted {
			cleanValue := strings.ReplaceAll(qm[i].price, ".", "")
			val, _ := strconv.ParseFloat(cleanValue, 64)

			// var finalEuro float64
			if qm[i].currency == "din" {
				val = val / 117.0
				qm[i].currency = "€"
			}
			qm[i].priceFloat = val
			qm[i].converted = true
		}
	}

	for n := 0; n < len(qm); n++ {
		for i := 0; i < len(qm)-1; i++ {
			if qm[i].priceFloat > qm[i+1].priceFloat {
				qm[i], qm[i+1] = qm[i+1], qm[i]
			}
		}
	}
	for i := range qm {
		qm[i].price = strconv.FormatFloat(qm[i].priceFloat, 'f', 0, 64)
	}

	return qm
}

func convertCurrencyToEUR(price float64) float64 {
	results := price / 117.0
	return results
}

// func convertCurrencyToRSD()
func initModel() model {
	// code providing spinner (loading screen tui)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("144"))

	ti := textinput.New()
	ti.Focus()
	ti.Placeholder = "input:"
	ti.CharLimit = 156
	ti.SetWidth(50)
	ti.SetVirtualCursor(false)
	// code providing listings and viewport:

	// l := list.New()
	return model{
		input:     ti,
		Keyword:   "",
		QModel:    []queryModel{},
		QSorted:   []queryModel{},
		spinner:   sp,
		sortPrice: false,
		selected:  make(map[int]struct{}),
		sort:      false,
	}
}

func openURL(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", "https://kupujemprodajem.com/" + url}
	case "darwin": // macOS
		cmd = "open"
		args = []string{"https://kupujemprodajem.com/" + url}
	default: // "linux", "freebsd", "openbsd", "netbsbsd"
		cmd = "xdg-open"
		args = []string{"https://kupujemprodajem.com/" + url}
	}
	return exec.Command(cmd, args...).Start()
}

func runQuery(keyword string) tea.Cmd {
	return func() tea.Msg {
		results := sendQuery(keyword)
		return queryResultMsg(results)
	}
}

func (m model) Init() tea.Cmd {
	CurrentArg := os.Args[1]
	if CurrentArg != "" {
		m.Keyword = CurrentArg
		query := runQuery(m.Keyword)

		return tea.Batch(
			m.spinner.Tick,
			query,
		)
	}
	return tea.Batch(
		m.spinner.Tick,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.canvas.width = msg.Width
		m.height = msg.Height - 6
		return m, nil
	case queryResultMsg:
		m.QModel = []queryModel(msg)
		m.viewport.Update(msg)

		m.QSorted = append([]queryModel{}, m.QModel...)
		m.QSorted = sortPrice(m.QSorted)
		m.QModelCopy = append([]queryModel{}, m.QModel...)
		return m, m.spinner.Tick

	case tea.KeyMsg:
		// temp := m.Panel
		switch m.Panel {
		case INPUT:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "ctrl+n":
				m.Panel = SEARCHED
			case "esc":
				m.input.Blur()
			case "enter":
				m.Keyword = m.input.Value()
				// m.viewport.Update(msg)
				return m, runQuery(m.Keyword)

			}
		case SEARCHED:
			switch msg.String() {
			case "i":
				_, typ, ok := strings.Cut(fmt.Sprintf("%T", msg), ".")
				if ok && unicode.IsUpper(rune(typ[0])) {
					cmds = append(cmds, tea.Printf("Received message: %T %+v", msg, msg))
				}
			case "q", "ctrl+c":
				return m, tea.Quit
			case "n":
				m.Panel = 1 - m.Panel

			case "m":
				m.Panel = SEARCHED
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--

					if m.cursor < m.startIndex {
						m.startIndex = m.cursor
					}
				}
			case "down", "j":
				if m.cursor < len(m.QModel)-1 {
					m.cursor++

					// m.cursor = 0
					// m.startIndex = 0
				}
				if m.cursor >= m.startIndex+m.height {
					m.startIndex = m.cursor - m.height + 1
				}
			case "s":
				m.sort = !m.sort
			case "l":
				m.sortPrice = !m.sortPrice
				if m.sortPrice == true {
					m.QModel = m.QSorted
				} else {
					m.QModel = m.QModelCopy
				}

			case "x", " ":
				if err := openURL(m.QModel[m.cursor].link); err != nil {
					fmt.Printf("error openining url %v\n", err)
				} else {
					m.selected[m.cursor] = struct{}{}
				}
			default:
				m.canvas.flip = !m.canvas.flip

			}
		}
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	if m.Panel == INPUT {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)

	}
	return m, tea.Batch(cmds...)
}

func (m model) View() tea.View {
	switch m.Panel {
	case INPUT:
		var b strings.Builder
		// b.WriteString("What are you buying:\n\n")
		b.WriteString(m.input.View())
		v := tea.NewView(b.String())
		v.Cursor = m.input.Cursor()

		// v.Cursor = m.input.Cursor()
		return v

	case SEARCHED:
		// b.WriteString("\n PRESS: [(q) quit] [(s) sort-order] [(l) sort-price] \n")
		// distance
		strAnim := fmt.Sprintf("\n %s  Searching KupujemProdajem... Please wait.\n", m.spinner.View())
		if len(m.QModel) == 0 {
			return tea.NewView(strAnim)
		}

		var b strings.Builder
		b.WriteString("\nWhat are you buying:\n")
		b.WriteString("\nsearch keyword" + m.Keyword + "\n\n")

		viewHeight := m.height
		if viewHeight <= 0 {
			viewHeight = 10
		}
		m.viewport.View()
		maxVisible := m.startIndex + m.height
		if maxVisible > len(m.QModel) || maxVisible < len(m.QModel) {
			maxVisible = len(m.QModel) - 1
		}
		end := min(m.startIndex+maxVisible+10, len(m.QModel))
		for i := m.startIndex; i < end; i++ {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}
			checked := " "
			if _, ok := m.selected[i]; ok {
				checked = "x"
			}
			// Using title instead of ID for better visibility
			pink := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
			gray := lipgloss.NewStyle().Foreground(lipgloss.Color("144"))
			// Build the line piece by piece

			if !m.sort {
				// 1. Format the string
				str1 := fmt.Sprintf("%s [%s] %s - %s%s", cursor, pink.Render(checked), gray.Render(m.QModel[i].title), gray.Render(m.QModel[i].currency), m.QModel[i].price)
				// 2. Render it and write it to the builder
				b.WriteString(str1 + "\n")
			} else {
				str2 := fmt.Sprintf("%s [%s] %s%s - %s", cursor, pink.Render(checked), gray.Render(m.QModel[i].currency), m.QModel[i].price, gray.Render(m.QModel[i].title))
				b.WriteString(str2 + "\n")
			}

		}
		b.WriteString("\n PRESS: [(q) quit] [(s) sort-order] [(l) sort-price] \n")
		v := tea.NewView(b.String())
		return v
	}
	return tea.View{}
}

func findChrome() string {
	var chrome string = ""
	homeUser, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	switch runtime.GOOS {
	case "windows":
		chrome = homeUser + "\\AppData\\Local\\Chromium\\Application\\chrome.exe"
	case "darwin": // macOS
		chrome = "/usr/bin/chromium"
	default: // "linux", "freebsd", "openbsd", "netbsbsd"
		chrome = "/usr/bin/chromium"

	}
	return chrome
}

func sendQuery(keyword string) []queryModel {
	searchURL := "https://www.kupujemprodajem.com/pretraga?keywords=" + url.QueryEscape(keyword)

	chromePath := findChrome()
	var Qbase []queryModel
	// if runtime.GOOS == "windows"
	url := launcher.New().Bin(chromePath).Leakless(false).MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()
	if keyword != "" {

		page := browser.MustPage(searchURL)
		page.MustWaitLoad()
		page.MustWaitIdle()

		html := page.MustHTML()
		// os.WriteFile("debug.html", []byte(html), 0o644)
		// fmt.Printf("html file saved")
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))

		// Parse HTML with goquery
		doc.Find("article").Each(func(i int, s *goquery.Selection) {
			title := strings.TrimSpace(
				s.Find("div[class*='adInfoHolder'] a div[class*='name']").Text(),
			)
			price := strings.TrimSpace(s.Find("div[class*='adPrice'] div div div[class*='inlinePrice']").Text())
			link, _ := s.Find("a[href]").Attr("href")
			// fmt.Printf("\n=== Item %d ===\n", i+1)
			if title != "" && price != "Kontakt" {
				parts := strings.Split(strings.TrimSpace(price), " ")
				qm := queryModel{
					Id:    i,
					title: title,
					link:  link,
				}
				if len(parts) == 2 {
					qm.price = parts[0]
					qm.currency = parts[1]

				}
				Qbase = append(Qbase, qm)
				// fmt.Printf("Title: %s\n", title) // Fixed: added title argument
				// fmt.Printf("Price: %s\n", price)
				// fmt.Printf("Link: https://www.kupujemprodajem.com%s\n", link)
			}
		})

	}
	return Qbase
}

func main() {
	p := tea.NewProgram(initModel())
	if _, err := p.Run(); err != nil {
		os.Exit(1)
		fmt.Printf("%s%sSYSTEM ERROR:%s %v\n", Bold, Red, Reset, err)
	}
	fmt.Printf(" ")
}

func checkIFEmpty(value string) bool {
	if value != "" {
		log.Fatal("string is empty")
		return true
	}
	return false
}
