package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

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

// JobsCacheEntry represents a cached job entry with timestamp
type JobsCacheEntry struct {
	Jobs      []models.Job
	Timestamp time.Time
}

// JobsCache represents the jobs cache with TTL
type JobsCache struct {
	mu      sync.RWMutex
	entries map[int64]JobsCacheEntry
	ttl     time.Duration
}

// NewJobsCache creates a new jobs cache
func NewJobsCache(ttl time.Duration) *JobsCache {
	return &JobsCache{
		entries: make(map[int64]JobsCacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves jobs from cache if not expired
func (c *JobsCache) Get(runID int64) ([]models.Job, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[runID]
	if !exists {
		return nil, false
	}

	if time.Since(entry.Timestamp) > c.ttl {
		return nil, false
	}

	return entry.Jobs, true
}

// Set stores jobs in cache with current timestamp
func (c *JobsCache) Set(runID int64, jobs []models.Job) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[runID] = JobsCacheEntry{
		Jobs:      jobs,
		Timestamp: time.Now(),
	}
}

// Cleanup removes expired entries
func (c *JobsCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for runID, entry := range c.entries {
		if now.Sub(entry.Timestamp) > c.ttl {
			delete(c.entries, runID)
		}
	}
}

// App represents the main application state
type App struct {
	// 検索機能
	searchInputMode    bool
	searchInputBuffer  string
	searchActiveQuery  string // 検索確定後もハイライト用
	searchMatchIndices []int  // 検索ヒット行番号リスト
	searchMatchIndex   int    // 現在のヒットインデックス

	// workflow file閲覧モード
	viewingWorkflowFile bool
	workflowFileContent string
	workflowFilePath    string
	workflowFileLoading bool
	workflowFileOffset  int // スクロール位置
	client              *github.Client
	owner               string
	repo                string

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

	// Cache and debounce
	jobsCache     *JobsCache
	debounceTimer *time.Timer
	pendingRunID  int64
	debounceMutex sync.Mutex

	// Log jump input mode(行ジャンプ入力モード)
	jumpInputMode   bool
	jumpInputBuffer string
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
		jobsCache:        NewJobsCache(10 * time.Minute),
	}
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	// Start periodic cache cleanup
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			a.jobsCache.Cleanup()
		}
	}()

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
	case workflowFileLoadedMsg:
		a.workflowFileLoading = false
		a.workflowFileContent = msg.content
		a.workflowFilePath = msg.path
		a.viewingWorkflowFile = true
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

	if a.viewingWorkflowFile {
		return a.renderWorkflowFileView()
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
	// --- グローバルキー ---
	switch {
	case key.Matches(msg, a.keyMap.Quit):
		return a, tea.Quit
	}

	// Workflow file view
	if a.viewingWorkflowFile {
		if msg.Type == tea.KeyEsc || key.Matches(msg, a.keyMap.Left) {
			a.viewingWorkflowFile = false
			a.workflowFileContent = ""
			a.workflowFilePath = ""
			a.workflowFileOffset = 0
			return a, nil
		}
		if a.workflowFileLoading { // ignore keys while loading
			return a, nil
		}

		lines := strings.Split(a.workflowFileContent, "\n")
		viewHeight := a.height - 4
		if viewHeight < 1 {
			viewHeight = 1
		}
		maxOffset := len(lines) - viewHeight
		if maxOffset < 0 {
			maxOffset = 0
		}

		switch {
		case key.Matches(msg, a.keyMap.Up):
			if a.workflowFileOffset > 0 {
				a.workflowFileOffset--
			}
		case key.Matches(msg, a.keyMap.Down):
			if a.workflowFileOffset < maxOffset {
				a.workflowFileOffset++
			}
		case key.Matches(msg, a.keyMap.PageUp):
			a.workflowFileOffset -= viewHeight
			if a.workflowFileOffset < 0 {
				a.workflowFileOffset = 0
			}
		case key.Matches(msg, a.keyMap.PageDown):
			a.workflowFileOffset += viewHeight
			if a.workflowFileOffset > maxOffset {
				a.workflowFileOffset = maxOffset
			}
		case key.Matches(msg, a.keyMap.Home) || msg.String() == "g":
			a.workflowFileOffset = 0
		case key.Matches(msg, a.keyMap.End) || msg.String() == "G":
			a.workflowFileOffset = maxOffset
		}
		return a, nil
	}

	// Logs view
	if a.viewState == WorkflowRunLogsView {
		// 検索入力モード
		if (msg.String() == "f" || key.Matches(msg, a.keyMap.Right)) && a.currentRun != nil {
			path := a.currentRun.Path
			ref := a.currentRun.HeadSha
			if path == "" && a.currentWorkflow != nil { // fallback
				path = a.currentWorkflow.Path
			}
			if path != "" && ref != "" {
				a.workflowFileLoading = true
				a.workflowFileContent = ""
				a.workflowFilePath = path
				a.viewingWorkflowFile = true
				return a, func() tea.Msg {
					content, err := a.client.GetWorkflowFileAtRef(a.owner, a.repo, path, ref)
					if err != nil {
						return workflowFileLoadedMsg{content: "Failed to fetch workflow file: " + err.Error(), path: path}
					}
					return workflowFileLoadedMsg{content: content, path: path}
				}
			}
		}

		if a.searchInputMode { // search input
			switch msg.Type {
			case tea.KeyRunes:
				a.searchInputBuffer += msg.String()
			case tea.KeyBackspace:
				if len(a.searchInputBuffer) > 0 {
					a.searchInputBuffer = a.searchInputBuffer[:len(a.searchInputBuffer)-1]
				}
			case tea.KeyEnter:
				// 検索して一致行リストを作成し、最初の一致行にジャンプ
				lines := strings.Split(a.logs, "\n")
				query := a.searchInputBuffer
				a.searchMatchIndices = nil
				for i, line := range lines {
					if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
						a.searchMatchIndices = append(a.searchMatchIndices, i)
					}
				}
				if len(a.searchMatchIndices) > 0 {
					a.searchMatchIndex = 0
					// 画面の先頭に来るように
					maxOffset := len(lines) - (a.height - 6)
					if maxOffset < 0 {
						maxOffset = 0
					}
					offset := a.searchMatchIndices[0]
					if offset > maxOffset {
						offset = maxOffset
					}
					a.logOffset = offset
				} else {
					a.searchMatchIndex = -1
				}
				a.searchInputMode = false
				a.searchActiveQuery = a.searchInputBuffer // ハイライト維持
			case tea.KeyEsc:
				a.searchInputMode = false
				a.searchInputBuffer = ""
				a.searchActiveQuery = "" // エスケープ時は必ずハイライトも消す
				a.searchMatchIndices = nil
				a.searchMatchIndex = -1
			}
			return a, nil
		}
		// ジャンプ入力モード
		if a.jumpInputMode {
			switch msg.Type {
			case tea.KeyRunes:
				r := msg.String()
				if r >= "0" && r <= "9" {
					a.jumpInputBuffer += r
				}
			case tea.KeyBackspace:
				if len(a.jumpInputBuffer) > 0 {
					a.jumpInputBuffer = a.jumpInputBuffer[:len(a.jumpInputBuffer)-1]
				}
			case tea.KeyEnter:
				if n, err := strconv.Atoi(a.jumpInputBuffer); err == nil && n > 0 {
					lines := strings.Split(a.logs, "\n")
					maxOffset := len(lines) - (a.height - 6)
					if maxOffset < 0 {
						maxOffset = 0
					}
					offset := n - 1
					if offset > maxOffset {
						offset = maxOffset
					}
					a.logOffset = offset
				}
				a.jumpInputMode = false
				a.jumpInputBuffer = ""
			case tea.KeyEsc:
				a.jumpInputMode = false
				a.jumpInputBuffer = ""
			}
			return a, nil
		}
		// /で検索入力モード開始
		if msg.String() == "/" {
			a.searchInputMode = true
			a.searchInputBuffer = ""
			return a, nil
		}
		// :でジャンプ入力モード開始
		if msg.String() == ":" {
			a.jumpInputMode = true
			a.jumpInputBuffer = ""
			return a, nil
		}
		switch {
		case key.Matches(msg, a.keyMap.Left):
			return a.goBack()
		case msg.Type == tea.KeyEsc:
			a.searchActiveQuery = "" // エスケープ時はハイライト消す
			a.searchMatchIndices = nil
			a.searchMatchIndex = -1
		// n: 次の検索ヒットへジャンプ
		case msg.String() == "n":
			if a.searchActiveQuery != "" && len(a.searchMatchIndices) > 0 {
				a.searchMatchIndex = (a.searchMatchIndex + 1) % len(a.searchMatchIndices)
				lines := strings.Split(a.logs, "\n")
				maxOffset := len(lines) - (a.height - 6)
				if maxOffset < 0 {
					maxOffset = 0
				}
				offset := a.searchMatchIndices[a.searchMatchIndex]
				if offset > maxOffset {
					offset = maxOffset
				}
				a.logOffset = offset
			}
		// Shift+n (N): 前の検索ヒットへジャンプ
		case msg.String() == "N":
			if a.searchActiveQuery != "" && len(a.searchMatchIndices) > 0 {
				a.searchMatchIndex = (a.searchMatchIndex - 1 + len(a.searchMatchIndices)) % len(a.searchMatchIndices)
				lines := strings.Split(a.logs, "\n")
				maxOffset := len(lines) - (a.height - 6)
				if maxOffset < 0 {
					maxOffset = 0
				}
				offset := a.searchMatchIndices[a.searchMatchIndex]
				if offset > maxOffset {
					offset = maxOffset
				}
				a.logOffset = offset
			}
		}
		return a.handleLogNavigation(msg)
	}

	// Other views
	switch {
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
		return a.handleEnter()
	case key.Matches(msg, a.keyMap.NextPage):
		return a.handleNextPage()
	case key.Matches(msg, a.keyMap.PrevPage):
		return a.handlePrevPage()
	}

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

		// If selection changed, load jobs for the new selection with debounce
		if a.allRunsList.Index() != oldIndex && len(a.allRuns) > 0 {
			if a.allRunsList.Index() < len(a.allRuns) {
				selectedRun := a.allRuns[a.allRunsList.Index()]
				a.scheduleJobsLoad(selectedRun.ID)
			}
		}
	case WorkflowListView:
		a.workflowList, cmd = a.workflowList.Update(msg)
		cmds = append(cmds, cmd)
	case WorkflowRunsView:
		oldIndex := a.runsList.Index()
		a.runsList, cmd = a.runsList.Update(msg)
		cmds = append(cmds, cmd)

		// If selection changed, load jobs for the new selection with debounce
		if a.runsList.Index() != oldIndex && len(a.workflowRuns) > 0 {
			if a.runsList.Index() < len(a.workflowRuns) {
				selectedRun := a.workflowRuns[a.runsList.Index()]
				a.scheduleJobsLoad(selectedRun.ID)
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

	highlightedLines := make([]string, len(visibleLines))
	lineNumberWidth := len(fmt.Sprintf("%d", len(lines))) // 桁数揃え
	stepGroupPrefix := "##[group]Run "

	// 区切り線の長さを計算
	// header(タイトル)やhelp分を除いた幅、行番号+区切り記号分も除く
	// 例: " 123 | " なら lineNumberWidth+3
	sepLen := a.width - (lineNumberWidth + 3) - 2 // 2は左右の余白分の目安
	if sepLen < 10 {
		sepLen = 10
	}
	sepStr := strings.Repeat("─", sepLen)

	// 検索ワードハイライト用
	var searchQuery string
	if a.searchInputMode && a.searchInputBuffer != "" {
		searchQuery = a.searchInputBuffer
	} else if a.searchActiveQuery != "" {
		searchQuery = a.searchActiveQuery
	}

	for i, line := range visibleLines {
		lineNum := start + i + 1
		// 行番号をつける
		prefix := fmt.Sprintf("%*d | ", lineNumberWidth, lineNum)

		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, stepGroupPrefix) {
			sep := lipgloss.NewStyle().Foreground(lipgloss.Color("36")).Bold(true).Render(sepStr)
			highlightedLines = append(highlightedLines, sep)
		}

		// 検索ワードがあれば黄色でハイライト
		renderedLine := a.applySimpleHighlight(line)
		if searchQuery != "" {
			idx := strings.Index(strings.ToLower(renderedLine), strings.ToLower(searchQuery))
			if idx >= 0 {
				before := renderedLine[:idx]
				match := renderedLine[idx : idx+len(searchQuery)]
				after := renderedLine[idx+len(searchQuery):]
				match = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true).Render(match)
				renderedLine = before + match + after
			}
		}
		highlightedLines = append(highlightedLines, prefix+renderedLine)
	}
	content := strings.Join(highlightedLines, "\n")

	// Prompt for search/jump input mode
	var inputPrompt string
	if a.searchInputMode {
		inputPrompt = a.styles.GetHelp().Render("/" + a.searchInputBuffer + "_  (Enter: search, n/N: next/prev match, Esc: reset)")
	} else if a.jumpInputMode {
		inputPrompt = a.styles.GetHelp().Render(":" + a.jumpInputBuffer + "_  (Enter to jump / Esc to cancel)")
	} else if a.searchActiveQuery != "" {
		inputPrompt = a.styles.GetHelp().Render("n/N: next/prev match, Esc: reset")
	}

	help := a.styles.GetHelp().Render("↑/↓: Scroll • PageUp/PageDown: Page UpDown • g/G: Top/Bottom • q: Quit • / to search :n to jump・ f|→: View workflow file")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		inputPrompt,
		help,
	)
}

func (a *App) renderWorkflowFileView() string {
	title := "Workflow File"
	if a.workflowFilePath != "" {
		title = fmt.Sprintf("Workflow File: %s", a.workflowFilePath)
	}
	header := a.styles.GetTitle().Render(title)
	var body string
	if a.workflowFileLoading {
		body = a.styles.GetStatusInProgress().Render("Loading workflow file...")
	} else if a.workflowFileContent == "" {
		body = a.styles.GetHelp().Render("(empty file)")
	} else {
		lines := strings.Split(a.workflowFileContent, "\n")
		viewHeight := a.height - 4 // header + help
		if viewHeight < 1 {
			viewHeight = 1
		}
		start := a.workflowFileOffset
		end := start + viewHeight
		if start > len(lines) {
			start = len(lines)
		}
		if end > len(lines) {
			end = len(lines)
		}
		visibleRaw := lines[start:end]
		digits := len(fmt.Sprintf("%d", len(lines)))
		visible := make([]string, len(visibleRaw))
		for i, raw := range visibleRaw {
			high := a.applyYAMLHighlight(raw)
			ln := start + i + 1
			visible[i] = fmt.Sprintf("%*d | %s", digits, ln, high)
		}
		body = lipgloss.NewStyle().Width(a.width - 4).Render(strings.Join(visible, "\n"))
	}
	help := a.styles.GetHelp().Render("Esc|←: Close • ↑/↓ PgUp/PgDn g/G: Scroll • q: Quit")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, help)
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

// workflow file load result
type workflowFileLoadedMsg struct {
	content string
	path    string
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

// scheduleJobsLoad schedules a debounced jobs load
func (a *App) scheduleJobsLoad(runID int64) {
	a.debounceMutex.Lock()
	defer a.debounceMutex.Unlock()

	// キャッシュから取得を試行
	if jobs, found := a.jobsCache.Get(runID); found {
		a.currentJobs = jobs
		return
	}

	// Cancel existing timer
	if a.debounceTimer != nil {
		a.debounceTimer.Stop()
	}

	a.pendingRunID = runID

	// Set new timer
	a.debounceTimer = time.AfterFunc(400*time.Millisecond, func() {
		// Execute the API call after debounce period
		a.executeJobsLoad(runID)
	})
}

// executeJobsLoad executes the actual jobs load
func (a *App) executeJobsLoad(runID int64) {
	a.debounceMutex.Lock()
	defer a.debounceMutex.Unlock()

	// Check if this is still the pending request
	if a.pendingRunID != runID {
		return
	}

	// キャッシュから再度確認（並行処理対策）
	if jobs, found := a.jobsCache.Get(runID); found {
		a.currentJobs = jobs
		return
	}

	// API呼び出し実行
	go func() {
		jobs, err := a.client.GetWorkflowRunJobs(a.owner, a.repo, runID)
		if err == nil {
			// キャッシュに保存
			a.jobsCache.Set(runID, jobs)
			a.currentJobs = jobs
		}
	}()
}

func (a *App) loadWorkflowRunJobs(runID int64) tea.Cmd {
	return tea.Cmd(func() tea.Msg {
		// キャッシュから取得を試行
		if jobs, found := a.jobsCache.Get(runID); found {
			return jobsLoadedMsg{jobs: jobs}
		}

		// キャッシュにない場合のみAPI呼び出し
		jobs, err := a.client.GetWorkflowRunJobs(a.owner, a.repo, runID)
		if err != nil {
			return errorMsg{err: err}
		}

		// キャッシュに保存
		a.jobsCache.Set(runID, jobs)

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

// applyYAMLHighlight provides simple inline YAML syntax highlighting.
func (a *App) applyYAMLHighlight(line string) string {
	trimmed := strings.TrimRight(line, "\r")
	if strings.TrimSpace(trimmed) == "" {
		return trimmed
	}

	// Separate comment portion
	comment := ""
	codePart := trimmed
	if idx := strings.Index(codePart, "#"); idx >= 0 {
		comment = codePart[idx:]
		codePart = codePart[:idx]
	}

	// Monokai Extended palette
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("197")).Bold(true)
	strStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("223"))
	boolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true)
	numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("148"))
	commentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("59")).Italic(true)

	// Key (supports leading spaces and list dash)
	keyRegex := regexp.MustCompile(`^([ \t-]*)([A-Za-z0-9_."'\-]+):(.*)$`)
	if m := keyRegex.FindStringSubmatch(codePart); m != nil {
		prefix, k, rest := m[1], m[2], m[3]
		codePart = prefix + keyStyle.Render(k) + ":" + rest
	}

	// Strings
	strRegex := regexp.MustCompile(`"[^"\\]*(?:\\.[^"\\]*)*"|'[^'\\]*(?:\\.[^'\\]*)*'`)
	codePart = strRegex.ReplaceAllStringFunc(codePart, func(s string) string { return strStyle.Render(s) })

	// Booleans / null
	boolRegex := regexp.MustCompile(`\b(true|false|null)\b`)
	codePart = boolRegex.ReplaceAllStringFunc(codePart, func(s string) string { return boolStyle.Render(s) })

	// Numbers (simpler: match standalone numeric tokens with optional % following)
	numRegex := regexp.MustCompile(`(^|[ \t:,\[\{])(-?\d+(?:\.\d+)?%?)([ \t,\]\}]|$)`)
	codePart = numRegex.ReplaceAllStringFunc(codePart, func(m string) string {
		sub := numRegex.FindStringSubmatch(m)
		if len(sub) < 4 {
			return m
		}
		return sub[1] + numStyle.Render(sub[2]) + sub[3]
	})

	if comment != "" {
		codePart += commentStyle.Render(comment)
	}
	return codePart
}
