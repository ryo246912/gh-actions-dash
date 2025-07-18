package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ryo246912/gh-actions-dash/internal/github"
	"github.com/ryo246912/gh-actions-dash/internal/logs"
	"github.com/ryo246912/gh-actions-dash/internal/models"
	"github.com/ryo246912/gh-actions-dash/internal/tui/components"
)

// ViewState represents the current view state
type ViewState int

const (
	AllRunsView ViewState = iota
	WorkflowListView
	WorkflowRunsView
	WorkflowRunLogsView
)

// App represents the main application state
type App struct {
	client *github.Client
	owner  string
	repo   string

	// UI state
	viewState ViewState
	keyMap    KeyMap
	styles    Styles
	help      help.Model

	// Data
	workflows       []models.Workflow
	workflowRuns    []models.WorkflowRun
	allRuns         []models.WorkflowRun
	currentWorkflow *models.Workflow
	currentRun      *models.WorkflowRun
	currentJobs     []models.Job
	logs            string

	// Lists
	workflowList list.Model
	runsList     list.Model
	allRunsList  list.Model

	// Preview panel
	previewPanel *components.PreviewPanel

	// Log processor
	logProcessor *logs.Processor

	// Scrollable content
	logOffset int

	// Dimensions
	width  int
	height int

	// Loading state
	loading bool
	err     error

	// Pagination state
	workflowsPage    int
	workflowsPerPage int
	workflowsTotal   int
	allRunsPage      int
	allRunsPerPage   int
	allRunsTotal     int
}

// NewApp creates a new TUI application
func NewApp(client *github.Client, owner, repo string) *App {
	keyMap := DefaultKeyMap()
	styles := DefaultStyles()

	// Create workflow list
	workflowList := list.New([]list.Item{}, components.NewWorkflowItemDelegate(styles), 0, 0)
	workflowList.Title = "Workflows"
	workflowList.SetShowStatusBar(false)
	workflowList.SetFilteringEnabled(false)
	workflowList.SetShowHelp(false) // Hide help to show more items
	workflowList.Styles.Title = styles.GetTitle()

	// Create runs list
	runsList := list.New([]list.Item{}, components.NewWorkflowRunItemDelegate(styles), 0, 0)
	runsList.Title = "Workflow Runs"
	runsList.SetShowStatusBar(false)
	runsList.SetFilteringEnabled(false)
	runsList.SetShowHelp(false) // Hide help to show more items
	runsList.Styles.Title = styles.GetTitle()

	// Create all runs list
	allRunsList := list.New([]list.Item{}, components.NewWorkflowRunItemDelegate(styles), 0, 0)
	allRunsList.Title = "All Workflow Runs"
	allRunsList.SetShowStatusBar(false)
	allRunsList.SetFilteringEnabled(false)
	allRunsList.SetShowHelp(false) // Hide help to show more items
	allRunsList.Styles.Title = styles.GetTitle()

	// Create preview panel
	previewPanel := components.NewPreviewPanel(styles)

	return &App{
		client:           client,
		owner:            owner,
		repo:             repo,
		viewState:        AllRunsView,
		keyMap:           keyMap,
		styles:           styles,
		help:             help.New(),
		workflowList:     workflowList,
		runsList:         runsList,
		allRunsList:      allRunsList,
		previewPanel:     previewPanel,
		logProcessor:     logs.NewProcessor(styles.GetContent()),
		loading:          true,
		workflowsPage:    1,
		workflowsPerPage: 100,
		allRunsPage:      1,
		allRunsPerPage:   100,
	}
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.loadAllRunsPaginated(),
		tea.EnterAltScreen,
	)
}

// Update handles messages and updates the application state
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.updateListSizes()
		return a, nil

	case tea.KeyMsg:
		return a.handleKeyMsg(msg)

	case workflowsLoadedMsg:
		a.workflows = msg.workflows
		a.loading = false
		a.updateWorkflowList()
		return a, nil

	case workflowRunsLoadedMsg:
		a.workflowRuns = msg.runs
		a.loading = false
		a.updateWorkflowRunsList()

		// Load jobs for the first run if available
		if len(a.workflowRuns) > 0 {
			return a, a.loadWorkflowRunJobs(a.workflowRuns[0].ID)
		}
		return a, nil

	case errorMsg:
		a.err = msg.err
		a.loading = false
		return a, nil

	case logsLoadedMsg:
		a.logs = msg.logs
		a.loading = false
		return a, nil

	case jobsLoadedMsg:
		a.currentJobs = msg.jobs
		return a, nil

	case allRunsLoadedMsg:
		a.allRuns = msg.runs
		a.loading = false
		a.updateAllRunsList()

		// Load jobs for the first run if available
		if len(a.allRuns) > 0 {
			return a, a.loadWorkflowRunJobs(a.allRuns[0].ID)
		}
		return a, nil

	case workflowsPaginatedLoadedMsg:
		a.workflows = msg.workflows
		a.workflowsTotal = msg.total
		a.workflowsPage = msg.page
		a.loading = false
		a.updateWorkflowList()
		return a, nil

	case allRunsPaginatedLoadedMsg:
		a.allRuns = msg.runs
		a.allRunsTotal = msg.total
		a.allRunsPage = msg.page
		a.loading = false
		a.updateAllRunsList()

		// Load jobs for the first run if available
		if len(a.allRuns) > 0 {
			return a, a.loadWorkflowRunJobs(a.allRuns[0].ID)
		}
		return a, nil
	}

	return a.updateLists(msg)
}

// handleNextPage handles next page navigation
func (a *App) handleNextPage() (tea.Model, tea.Cmd) {
	switch a.viewState {
	case WorkflowListView:
		if a.workflowsPage*a.workflowsPerPage < a.workflowsTotal {
			a.workflowsPage++
			a.loading = true
			return a, a.loadWorkflowsPaginated()
		}
	case AllRunsView:
		if a.allRunsPage*a.allRunsPerPage < a.allRunsTotal {
			a.allRunsPage++
			a.loading = true
			return a, a.loadAllRunsPaginated()
		}
	}
	return a, nil
}

// handlePrevPage handles previous page navigation
func (a *App) handlePrevPage() (tea.Model, tea.Cmd) {
	switch a.viewState {
	case WorkflowListView:
		if a.workflowsPage > 1 {
			a.workflowsPage--
			a.loading = true
			return a, a.loadWorkflowsPaginated()
		}
	case AllRunsView:
		if a.allRunsPage > 1 {
			a.allRunsPage--
			a.loading = true
			return a, a.loadAllRunsPaginated()
		}
	}
	return a, nil
}

// getPaginationInfo returns pagination information string
func (a *App) getPaginationInfo(page, total, perPage int) string {
	totalPages := (total + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	return fmt.Sprintf("Page %d of %d (%d items)", page, totalPages, total)
}

// View renders the application
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "Loading..."
	}

	if a.err != nil {
		return a.renderError(a.err)
	}

	if a.loading {
		return a.styles.GetStatusInProgress().Render("Loading...")
	}

	switch a.viewState {
	case AllRunsView:
		return a.renderAllRunsView()
	case WorkflowListView:
		return a.renderWorkflowListView()
	case WorkflowRunsView:
		return a.renderWorkflowRunsView()
	case WorkflowRunLogsView:
		return a.renderWorkflowRunLogsView()
	default:
		return "Unknown view state"
	}
}

// handleKeyMsg handles keyboard input
func (a *App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keyMap.Quit):
		return a, tea.Quit

	case key.Matches(msg, a.keyMap.Back):
		return a.goBack()

	case key.Matches(msg, a.keyMap.Enter):
		return a.handleEnter()

	case key.Matches(msg, a.keyMap.Refresh):
		return a.refresh()

	case msg.String() == "w":
		return a.switchToWorkflowsView()

	case msg.String() == "a":
		return a.switchToAllRunsView()

	case key.Matches(msg, a.keyMap.Right):
		// l key acts as Enter for forward navigation
		return a.handleEnter()

	case key.Matches(msg, a.keyMap.Left):
		// h key acts as Esc for backward navigation
		return a.goBack()

	case key.Matches(msg, a.keyMap.NextPage):
		return a.handleNextPage()

	case key.Matches(msg, a.keyMap.PrevPage):
		return a.handlePrevPage()

	}

	// Handle log scrolling for logs view
	if a.viewState == WorkflowRunLogsView {
		return a.handleLogNavigation(msg)
	}

	// Pass navigation keys to the active list
	return a.updateLists(msg)
}

// switchToWorkflowsView switches to the workflows view
func (a *App) switchToWorkflowsView() (tea.Model, tea.Cmd) {
	if a.viewState == AllRunsView {
		a.viewState = WorkflowListView
		a.loading = true
		return a, a.loadWorkflowsPaginated()
	}
	return a, nil
}

// switchToAllRunsView switches to the all runs view
func (a *App) switchToAllRunsView() (tea.Model, tea.Cmd) {
	if a.viewState == WorkflowListView || a.viewState == WorkflowRunsView {
		a.viewState = AllRunsView
		a.currentWorkflow = nil
		a.loading = true
		return a, a.loadAllRunsPaginated()
	}
	return a, nil
}

// handleEnter handles the enter key
func (a *App) handleEnter() (tea.Model, tea.Cmd) {
	switch a.viewState {
	case AllRunsView:
		if len(a.allRuns) == 0 {
			return a, nil // No runs available
		}
		if item, ok := a.allRunsList.SelectedItem().(components.WorkflowRunItem); ok {
			a.currentRun = &item.Run
			a.viewState = WorkflowRunLogsView
			a.loading = true
			a.logOffset = 0
			a.logs = ""
			return a, a.loadWorkflowRunLogs(item.Run.ID)
		}
	case WorkflowListView:
		if len(a.workflows) == 0 {
			return a, nil // No workflows available
		}
		if item, ok := a.workflowList.SelectedItem().(components.WorkflowItem); ok {
			a.currentWorkflow = &item.Workflow
			a.viewState = WorkflowRunsView
			a.loading = true
			return a, a.loadWorkflowRuns(item.Workflow.ID)
		}
	case WorkflowRunsView:
		if len(a.workflowRuns) == 0 {
			return a, nil // No workflow runs available
		}
		if item, ok := a.runsList.SelectedItem().(components.WorkflowRunItem); ok {
			a.currentRun = &item.Run
			a.viewState = WorkflowRunLogsView
			a.loading = true
			a.logOffset = 0
			a.logs = ""
			return a, a.loadWorkflowRunLogs(item.Run.ID)
		}
	}

	return a, nil
}

// goBack handles the back action
func (a *App) goBack() (tea.Model, tea.Cmd) {
	switch a.viewState {
	case WorkflowListView:
		a.viewState = AllRunsView
		return a, nil
	case WorkflowRunsView:
		a.viewState = WorkflowListView
		return a, nil
	case WorkflowRunLogsView:
		if a.currentWorkflow != nil {
			a.viewState = WorkflowRunsView
		} else {
			a.viewState = AllRunsView
		}
		return a, nil
	}

	return a, nil
}

// refresh refreshes the current view
func (a *App) refresh() (tea.Model, tea.Cmd) {
	a.loading = true

	switch a.viewState {
	case AllRunsView:
		return a, a.loadAllRunsPaginated()
	case WorkflowListView:
		return a, a.loadWorkflowsPaginated()
	case WorkflowRunsView:
		if a.currentWorkflow != nil {
			return a, a.loadWorkflowRuns(a.currentWorkflow.ID)
		}
	case WorkflowRunLogsView:
		if a.currentRun != nil {
			a.logOffset = 0
			a.logs = ""
			return a, a.loadWorkflowRunLogs(a.currentRun.ID)
		}
	}

	return a, nil
}

// updateLists updates the list components
func (a *App) updateLists(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch a.viewState {
	case AllRunsView:
		oldIndex := a.allRunsList.Index()
		a.allRunsList, cmd = a.allRunsList.Update(msg)
		cmds = append(cmds, cmd)

		// If selection changed, load jobs for the new selection
		if a.allRunsList.Index() != oldIndex && len(a.allRuns) > 0 {
			if a.allRunsList.Index() < len(a.allRuns) {
				selectedRun := a.allRuns[a.allRunsList.Index()]
				cmds = append(cmds, a.loadWorkflowRunJobs(selectedRun.ID))
			}
		}
	case WorkflowListView:
		a.workflowList, cmd = a.workflowList.Update(msg)
		cmds = append(cmds, cmd)
	case WorkflowRunsView:
		oldIndex := a.runsList.Index()
		a.runsList, cmd = a.runsList.Update(msg)
		cmds = append(cmds, cmd)

		// If selection changed, load jobs for the new selection
		if a.runsList.Index() != oldIndex && len(a.workflowRuns) > 0 {
			if a.runsList.Index() < len(a.workflowRuns) {
				selectedRun := a.workflowRuns[a.runsList.Index()]
				cmds = append(cmds, a.loadWorkflowRunJobs(selectedRun.ID))
			}
		}
	}

	return a, tea.Batch(cmds...)
}

// updateListSizes updates the list sizes based on window dimensions
func (a *App) updateListSizes() {
	switch a.viewState {
	case WorkflowRunsView, AllRunsView:
		// 2-column layout for workflow runs view and all runs view
		// Use approximately 60% for list and 40% for preview to maximize usage
		listWidth := (a.width*3)/5 - 2 // 60% minus small margin
		listHeight := a.height - 6
		previewWidth := (a.width*2)/5 - 1 // 40% minus small margin
		previewHeight := a.height - 4

		a.runsList.SetSize(listWidth, listHeight)
		a.allRunsList.SetSize(listWidth, listHeight)
		a.previewPanel.SetSize(previewWidth, previewHeight)
	case WorkflowListView:
		// 2-column layout for workflow list view
		// Use approximately 60% for list and 40% for preview to maximize usage
		listWidth := (a.width*3)/5 - 1    // 60% minus small margin
		listHeight := a.height - 4        // Reduce margin to show more items
		previewWidth := (a.width*2)/5 - 1 // 40% minus small margin
		previewHeight := a.height - 4     // Account for header and margins

		// Ensure minimum sizes to prevent display issues
		if listWidth < 20 {
			listWidth = 20
		}
		if listHeight < 5 {
			listHeight = 5
		}
		if previewWidth < 15 {
			previewWidth = 15
		}
		if previewHeight < 5 {
			previewHeight = 5
		}

		a.workflowList.SetSize(listWidth, listHeight)
		a.previewPanel.SetSize(previewWidth, previewHeight)
	default:
		// Full width for other views (logs view)
		listWidth := a.width - 4
		listHeight := a.height - 6

		a.workflowList.SetSize(listWidth, listHeight)
		a.runsList.SetSize(listWidth, listHeight)
		a.allRunsList.SetSize(listWidth, listHeight)
	}
}

// updateWorkflowList updates the workflow list items
func (a *App) updateWorkflowList() {
	items := make([]list.Item, len(a.workflows))
	for i, workflow := range a.workflows {
		items[i] = components.WorkflowItem{Workflow: workflow}
	}
	a.workflowList.SetItems(items)

	// Update list title to show count
	if len(a.workflows) == 0 {
		a.workflowList.Title = "Workflows (No workflows found)"
	} else {
		a.workflowList.Title = fmt.Sprintf("Workflows (%d)", len(a.workflows))
	}
}

// updateWorkflowRunsList updates the workflow runs list items
func (a *App) updateWorkflowRunsList() {
	items := make([]list.Item, len(a.workflowRuns))
	for i, run := range a.workflowRuns {
		items[i] = components.WorkflowRunItem{Run: run}
	}
	a.runsList.SetItems(items)

	// Update list title to show count
	if len(a.workflowRuns) == 0 {
		a.runsList.Title = "Workflow Runs (No runs found)"
	} else {
		a.runsList.Title = fmt.Sprintf("Workflow Runs (%d)", len(a.workflowRuns))
	}
}

// updateAllRunsList updates the all runs list items
func (a *App) updateAllRunsList() {
	items := make([]list.Item, len(a.allRuns))
	for i, run := range a.allRuns {
		items[i] = components.WorkflowRunItem{Run: run}
	}
	a.allRunsList.SetItems(items)

	// Update list title to show count
	if len(a.allRuns) == 0 {
		a.allRunsList.Title = "All Workflow Runs (No runs found)"
	} else {
		a.allRunsList.Title = fmt.Sprintf("All Workflow Runs (%d)", len(a.allRuns))
	}
}

// renderWorkflowListView renders the workflow list view
func (a *App) renderWorkflowListView() string {
	header := a.styles.GetTitle().Render(fmt.Sprintf("GitHub Actions - %s/%s", a.owner, a.repo))

	help := a.styles.GetHelp().Render("Enter: View runs • a: All runs • r: Refresh • n: Next page • p: Prev page • q: Quit")

	// Pagination info
	paginationInfo := ""
	if a.workflowsTotal > 0 {
		paginationInfo = a.styles.GetHelp().Render(a.getPaginationInfo(a.workflowsPage, a.workflowsTotal, a.workflowsPerPage))
	}

	// Left side - workflow list
	var leftMainContent string
	if len(a.workflows) == 0 {
		emptyMessage := a.styles.GetHelp().Render("📋 このリポジトリにはGitHub Actions ワークフローがありません")
		emptyDetails := a.styles.GetHelp().Render("💡 .github/workflows/ ディレクトリにワークフローファイルを作成してください")
		leftMainContent = lipgloss.JoinVertical(
			lipgloss.Left,
			"",
			emptyMessage,
			emptyDetails,
			"",
		)
	} else {
		listView := a.workflowList.View()
		leftMainContent = listView
	}

	leftContentParts := []string{header, leftMainContent}
	if paginationInfo != "" {
		leftContentParts = append(leftContentParts, paginationInfo)
	}
	leftContentParts = append(leftContentParts, help)

	leftContent := lipgloss.JoinVertical(
		lipgloss.Left,
		leftContentParts...,
	)

	// Right side - preview panel
	var selectedWorkflow *models.Workflow
	if len(a.workflows) > 0 && a.workflowList.Index() < len(a.workflows) {
		selectedWorkflow = &a.workflows[a.workflowList.Index()]
	}

	rightContent := a.previewPanel.RenderWorkflowPreview(selectedWorkflow)

	// Create a container that places preview panel at the right edge
	previewWidth := (a.width * 2) / 5
	leftWidth := a.width - previewWidth

	// Ensure left content takes up the remaining space
	leftContainer := lipgloss.NewStyle().Width(leftWidth).Render(leftContent)

	// Right align the preview panel at the edge
	rightContainer := lipgloss.NewStyle().Width(previewWidth).AlignHorizontal(lipgloss.Right).Render(rightContent)

	mainContent := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftContainer,
		rightContainer,
	)

	return a.styles.Base.Render(mainContent)
}

// renderAllRunsView renders the all runs view (time-ordered)
func (a *App) renderAllRunsView() string {
	headerText := fmt.Sprintf("All Workflow Runs - %s/%s", a.owner, a.repo)
	header := a.styles.GetTitle().Render(headerText)

	help := a.styles.GetHelp().Render("Enter: View logs • w: Workflows • r: Refresh • n: Next page • p: Prev page • q: Quit")

	// Pagination info
	paginationInfo := ""
	if a.allRunsTotal > 0 {
		paginationInfo = a.styles.GetHelp().Render(a.getPaginationInfo(a.allRunsPage, a.allRunsTotal, a.allRunsPerPage))
	}

	// Left side - all runs list
	var leftMainContent string
	if len(a.allRuns) == 0 {
		emptyMessage := a.styles.GetHelp().Render("📋 このリポジトリには実行されたワークフローがありません")
		emptyDetails := a.styles.GetHelp().Render("💡 ワークフローを実行するか、トリガー条件を満たしてください")
		leftMainContent = lipgloss.JoinVertical(
			lipgloss.Left,
			"",
			emptyMessage,
			emptyDetails,
			"",
		)
	} else {
		// Add table header
		tableHeader := a.styles.GetHelp().Render("Name                     Status         Branch             Actor           PR           Duration Time")
		listView := a.allRunsList.View()
		leftMainContent = lipgloss.JoinVertical(
			lipgloss.Left,
			tableHeader,
			listView,
		)
	}

	leftContentParts := []string{header, leftMainContent}
	if paginationInfo != "" {
		leftContentParts = append(leftContentParts, paginationInfo)
	}
	leftContentParts = append(leftContentParts, help)

	leftContent := lipgloss.JoinVertical(
		lipgloss.Left,
		leftContentParts...,
	)

	// Right side - preview panel
	var selectedRun *models.WorkflowRun
	if len(a.allRuns) > 0 && a.allRunsList.Index() < len(a.allRuns) {
		selectedRun = &a.allRuns[a.allRunsList.Index()]
	}

	rightContent := a.previewPanel.RenderWorkflowRunPreview(selectedRun, a.currentJobs)

	// Create a container that places preview panel at the right edge
	previewWidth := (a.width * 2) / 5
	leftWidth := a.width - previewWidth

	// Ensure left content takes up the remaining space
	leftContainer := lipgloss.NewStyle().Width(leftWidth).Render(leftContent)

	// Right align the preview panel at the edge
	rightContainer := lipgloss.NewStyle().Width(previewWidth).AlignHorizontal(lipgloss.Right).Render(rightContent)

	mainContent := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftContainer,
		rightContainer,
	)

	return a.styles.Base.Render(mainContent)
}

// renderWorkflowRunsView renders the workflow runs view
func (a *App) renderWorkflowRunsView() string {
	title := fmt.Sprintf("Workflow Runs - %s", a.currentWorkflow.Name)
	header := a.styles.GetTitle().Render(title)

	help := a.styles.GetHelp().Render("Enter: View logs • Esc: Back • a: All runs • r: Refresh • q: Quit")

	// Left side - workflow runs list
	var leftMainContent string
	if len(a.workflowRuns) == 0 {
		emptyMessage := a.styles.GetHelp().Render("📋 このワークフローには実行履歴がありません")
		emptyDetails := a.styles.GetHelp().Render("💡 ワークフローを手動実行するか、トリガー条件を満たしてください")
		leftMainContent = lipgloss.JoinVertical(
			lipgloss.Left,
			"",
			emptyMessage,
			emptyDetails,
			"",
		)
	} else {
		// Add table header
		tableHeader := a.styles.GetHelp().Render("Name                     Status         Branch             Actor           PR           Duration Time")
		listView := a.runsList.View()
		leftMainContent = lipgloss.JoinVertical(
			lipgloss.Left,
			tableHeader,
			listView,
		)
	}

	leftContent := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		leftMainContent,
		help,
	)

	// Right side - preview panel
	var selectedRun *models.WorkflowRun
	if len(a.workflowRuns) > 0 && a.runsList.Index() < len(a.workflowRuns) {
		selectedRun = &a.workflowRuns[a.runsList.Index()]
	}

	rightContent := a.previewPanel.RenderWorkflowRunPreview(selectedRun, a.currentJobs)

	// Create a container that places preview panel at the right edge
	previewWidth := (a.width * 2) / 5
	leftWidth := a.width - previewWidth

	// Ensure left content takes up the remaining space
	leftContainer := lipgloss.NewStyle().Width(leftWidth).Render(leftContent)

	// Right align the preview panel at the edge
	rightContainer := lipgloss.NewStyle().Width(previewWidth).AlignHorizontal(lipgloss.Right).Render(rightContent)

	mainContent := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftContainer,
		rightContainer,
	)

	return a.styles.Base.Render(mainContent)
}

// renderWorkflowRunLogsView renders the workflow run logs view
func (a *App) renderWorkflowRunLogsView() string {
	if a.currentRun == nil {
		return "No run selected"
	}

	title := fmt.Sprintf("Logs - Run #%d", a.currentRun.RunNumber)
	header := a.styles.GetTitle().Render(title)

	if a.logs == "" {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			a.styles.GetStatusInProgress().Render("Loading logs..."),
		)
	}

	// Split logs into lines for scrolling
	lines := strings.Split(a.logs, "\n")
	viewHeight := a.height - 6 // Account for header and help

	// Calculate visible lines
	start := a.logOffset
	end := start + viewHeight
	if end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		start = len(lines)
	}

	visibleLines := lines[start:end]

	// Apply simple highlighting to lines
	highlightedLines := make([]string, len(visibleLines))
	for i, line := range visibleLines {
		highlightedLines[i] = a.applySimpleHighlight(line)
	}
	content := strings.Join(highlightedLines, "\n")

	help := a.styles.GetHelp().Render("↑/↓: Scroll • PageUp/PageDown: Page • Home/End: Top/Bottom • Esc: Back • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		help,
	)
}

// Messages
type workflowsLoadedMsg struct {
	workflows []models.Workflow
}

type workflowRunsLoadedMsg struct {
	runs []models.WorkflowRun
}

type errorMsg struct {
	err error
}

type logsLoadedMsg struct {
	logs string
}

type jobsLoadedMsg struct {
	jobs []models.Job
}

type allRunsLoadedMsg struct {
	runs []models.WorkflowRun
}

type workflowsPaginatedLoadedMsg struct {
	workflows []models.Workflow
	total     int
	page      int
}

type allRunsPaginatedLoadedMsg struct {
	runs  []models.WorkflowRun
	total int
	page  int
}

// Commands
func (a *App) loadWorkflowsPaginated() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		workflows, total, err := a.client.GetWorkflowsPaginated(a.owner, a.repo, a.workflowsPage, a.workflowsPerPage)
		if err != nil {
			return errorMsg{err: err}
		}
		return workflowsPaginatedLoadedMsg{workflows: workflows, total: total, page: a.workflowsPage}
	})
}

func (a *App) loadAllRunsPaginated() tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		allRuns, total, err := a.client.GetAllWorkflowRunsPaginated(a.owner, a.repo, a.allRunsPage, a.allRunsPerPage)
		if err != nil {
			return errorMsg{err: err}
		}
		return allRunsPaginatedLoadedMsg{runs: allRuns, total: total, page: a.allRunsPage}
	})
}

func (a *App) loadWorkflowRuns(workflowID int64) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		runs, err := a.client.GetWorkflowRuns(a.owner, a.repo, workflowID)
		if err != nil {
			return errorMsg{err: err}
		}
		return workflowRunsLoadedMsg{runs: runs}
	})
}

func (a *App) loadWorkflowRunLogs(runID int64) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		logs, err := a.client.GetWorkflowRunLogs(a.owner, a.repo, runID)
		if err != nil {
			return errorMsg{err: err}
		}
		return logsLoadedMsg{logs: logs}
	})
}

func (a *App) loadWorkflowRunJobs(runID int64) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		jobs, err := a.client.GetWorkflowRunJobs(a.owner, a.repo, runID)
		if err != nil {
			return errorMsg{err: err}
		}
		return jobsLoadedMsg{jobs: jobs}
	})
}

// handleLogNavigation handles navigation in the logs view
func (a *App) handleLogNavigation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.logs == "" {
		return a, nil
	}

	lines := strings.Split(a.logs, "\n")
	viewHeight := a.height - 6
	maxOffset := len(lines) - viewHeight
	if maxOffset < 0 {
		maxOffset = 0
	}

	switch {
	case key.Matches(msg, a.keyMap.Up):
		if a.logOffset > 0 {
			a.logOffset--
		}
	case key.Matches(msg, a.keyMap.Down):
		if a.logOffset < maxOffset {
			a.logOffset++
		}
	case key.Matches(msg, a.keyMap.PageUp):
		a.logOffset -= viewHeight
		if a.logOffset < 0 {
			a.logOffset = 0
		}
	case key.Matches(msg, a.keyMap.PageDown):
		a.logOffset += viewHeight
		if a.logOffset > maxOffset {
			a.logOffset = maxOffset
		}
	case key.Matches(msg, a.keyMap.Home):
		a.logOffset = 0
	case key.Matches(msg, a.keyMap.End):
		a.logOffset = maxOffset
	}

	return a, nil
}

// applySimpleHighlight applies simple color highlighting to log lines without borders
func (a *App) applySimpleHighlight(line string) string {
	// Only apply color changes, no borders or complex styling
	trimmedLine := strings.TrimSpace(line)

	// GitHub Actions commands - blue color
	if strings.Contains(trimmedLine, "[command]") {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true).Render(line) // Bold blue
	}

	// GitHub Actions group commands - purple color
	if strings.Contains(trimmedLine, "##[group]") || strings.Contains(trimmedLine, "##[endgroup]") {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("129")).Bold(true).Render(line) // Bold purple
	}

	// GitHub Actions error commands - red color
	if strings.Contains(trimmedLine, "##[error]") {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(line) // Bold red
	}

	// GitHub Actions warning commands - yellow color
	if strings.Contains(trimmedLine, "##[warning]") {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true).Render(line) // Bold yellow
	}

	// Return line as-is if no pattern matches
	return line
}

// renderError renders an error message with details
func (a *App) renderError(err error) string {
	if githubErr, ok := err.(*github.GitHubError); ok {
		var content strings.Builder

		// Main error message
		content.WriteString(a.styles.StatusFailure.Render(fmt.Sprintf("❌ %s", githubErr.Message)))
		content.WriteString("\n\n")

		// Details
		if githubErr.Details != "" {
			content.WriteString(a.styles.GetHelp().Render(fmt.Sprintf("💡 %s", githubErr.Details)))
			content.WriteString("\n\n")
		}

		// Error type specific suggestions
		switch githubErr.Type {
		case github.ErrorTypeAuth:
			content.WriteString(a.styles.GetHelp().Render("🔧 解決方法:"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  1. gh auth login を実行してGitHubにログインする"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  2. gh auth status でログイン状態を確認する"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  3. トークンに必要な権限があることを確認する"))
		case github.ErrorTypePermission:
			content.WriteString(a.styles.GetHelp().Render("🔧 解決方法:"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  1. リポジトリが存在することを確認する"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  2. リポジトリにアクセス権限があることを確認する"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  3. プライベートリポジトリの場合は適切な権限を設定する"))
		case github.ErrorTypeNotFound:
			content.WriteString(a.styles.GetHelp().Render("🔧 解決方法:"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  1. リポジトリ名とオーナー名を確認する"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  2. リポジトリが存在することを確認する"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  3. タイポがないか確認する"))
		case github.ErrorTypeNetwork:
			content.WriteString(a.styles.GetHelp().Render("🔧 解決方法:"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  1. インターネット接続を確認する"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  2. プロキシ設定を確認する"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  3. しばらく待ってから再試行する"))
		case github.ErrorTypeRateLimit:
			content.WriteString(a.styles.GetHelp().Render("🔧 解決方法:"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  1. しばらく待ってから再試行する"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  2. 認証済みトークンを使用する"))
		default:
			content.WriteString(a.styles.GetHelp().Render("🔧 一般的な解決方法:"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  1. しばらく待ってから再試行する"))
			content.WriteString("\n")
			content.WriteString(a.styles.GetHelp().Render("  2. GitHub CLI の更新を確認する"))
		}

		content.WriteString("\n\n")
		content.WriteString(a.styles.GetHelp().Render("r: 再試行 • q: 終了"))

		return a.styles.GetContent().Render(content.String())
	}

	// Fallback for non-GitHub errors
	return a.styles.StatusFailure.Render(fmt.Sprintf("❌ エラー: %s", err.Error()))
}
