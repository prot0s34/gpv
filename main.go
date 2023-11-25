// main.go
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/xanzy/go-gitlab"
)

var (
	gitlabClient *gitlab.Client
	token        string
)

func init() {
	token := os.Getenv("GITLAB_PERSONAL_TOKEN")
	if token == "" {
		fmt.Println("Please set GITLAB_PERSONAL_TOKEN environment variable.")
		os.Exit(1)
	}

	gitlabURL := os.Getenv("GITLAB_URL")
	if gitlabURL == "" {
		gitlabURL = "https://gitlab.com"
	}

	// Initialize GitLab client and handle errors
	var err error
	gitlabClient, err = gitlab.NewClient(token, gitlab.WithBaseURL(gitlabURL+"/api/v4"))
	if err != nil {
		fmt.Println("Error creating GitLab client:", err)
		os.Exit(1)
	}

	// Fetch and display some data using the gitlabClient variable
	groups, _, err := gitlabClient.Groups.ListGroups(&gitlab.ListGroupsOptions{})
	if err != nil {
		fmt.Println("Error fetching groups:", err)
		os.Exit(1)
	}

	//  debug purposes
	fmt.Println("Connecting to Instance:", gitlabURL)
	for _, group := range groups {
		fmt.Println("Group:", group.Name)
	}

}

func main() {
	app := tview.NewApplication()

	if err := app.SetRoot(buildTree(app), true).Run(); err != nil {
		fmt.Println("Error:", err)
	}
}

func buildTree(app *tview.Application) *tview.TreeView {
	root := tview.NewTreeNode("GitLab Pipelines").
		SetColor(tcell.ColorYellow).
		SetSelectable(false)

	tree := tview.NewTreeView().
		SetRoot(root).
		SetCurrentNode(root).
		SetTopLevel(1).
		SetGraphicsColor(tcell.ColorGreen)

	tree.SetSelectedFunc(func(node *tview.TreeNode) {
		// Handle selection logic here
		projectName := node.GetText()
		if strings.HasPrefix(projectName, "Project: ") {
			showPipelines(app, node)
		}
	})

	root.AddChild(buildGroups())

	return tree
}

func buildGroups() *tview.TreeNode {
	root := tview.NewTreeNode("Groups").
		SetColor(tcell.ColorYellow)

	groups, _, err := gitlabClient.Groups.ListGroups(&gitlab.ListGroupsOptions{})
	if err != nil {
		fmt.Println("Error fetching groups:", err)
		return root
	}

	for _, group := range groups {
		groupNode := tview.NewTreeNode("Group: " + group.Name).
			SetColor(tcell.ColorWhite)
		root.AddChild(groupNode)

		projects, _, err := gitlabClient.Groups.ListGroupProjects(group.ID, &gitlab.ListGroupProjectsOptions{})
		if err != nil {
			fmt.Println("Error fetching projects for group", group.Name, ":", err)
			continue
		}

		for _, project := range projects {
			projectNode := tview.NewTreeNode("Project: " + project.Name).
				SetColor(tcell.ColorBlue).
				SetReference(fmt.Sprintf("%d", project.ID)) // Convert project ID to string
			groupNode.AddChild(projectNode)
		}
	}

	return root
}

func showPipelines(app *tview.Application, projectNode *tview.TreeNode) {
	// Extract the project ID from the reference
	projectID, ok := projectNode.GetReference().(string)
	if !ok {
		fmt.Println("Invalid project reference")
		return
	}

	// Fetch branches for the selected project
	branches, _, err := gitlabClient.Branches.ListBranches(projectID, &gitlab.ListBranchesOptions{})
	if err != nil {
		fmt.Println("Error fetching branches for project", projectID, ":", err)
		return
	}

	// Create a new modal to select the branch
	var modal *tview.Modal // Declare modal outside SetDoneFunc
	modal = tview.NewModal().
		SetText("Select Branch").
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex >= 0 && buttonIndex < len(branches) {
				// User selected a branch, fetch pipelines for that branch
				selectedBranch := branches[buttonIndex].Name
				fetchAndShowPipelines(app, projectID, selectedBranch)
			} else {
				// User closed the modal without selecting a branch
				app.SetFocus(modal) // Focus on the modal
			}
		})

		// Add buttons for each branch
	var buttons []string
	for _, branch := range branches {
		branchName := branch.Name
		buttons = append(buttons, fmt.Sprintf("%s", branchName))
	}

	// Add a cancel button
	buttons = append(buttons, "Cancel")

	// Set the buttons for the modal
	modal.AddButtons(buttons)

	// Set the root of the application to the modal
	app.SetRoot(modal, true).SetFocus(modal) // Add buttons for each branch
}

func fetchAndShowPipelines(app *tview.Application, projectID, branch string) {
	// Fetch and display pipeline information for the selected project and branch
	projectPipelines, _, err := gitlabClient.Pipelines.ListProjectPipelines(projectID, &gitlab.ListProjectPipelinesOptions{
		Ref: &branch,
	})
	if err != nil {
		fmt.Println("Error fetching pipelines for project", projectID, "and branch", branch, ":", err)
		return
	}

	// Create a new tview.List to display pipeline information
	pipelineList := tview.NewList().ShowSecondaryText(false)

	for _, pipeline := range projectPipelines {
		// Format pipeline information as a string
		pipelineInfo := fmt.Sprintf("Pipeline ID: %d \nStatus: %s \nRef: %s \nSource: %s \nUpdated At: %s \n",
			pipeline.ID, pipeline.Status, pipeline.Ref, pipeline.Source, pipeline.UpdatedAt.Format("2006-01-02 15:04:05"))

		// Add the pipeline information to the list
		pipelineList.AddItem(pipelineInfo, "", 0, func() {
			// Pass the 'app' parameter explicitly to the fetchAndShowJobs function
			fetchAndShowJobs(app, projectID, fmt.Sprintf("%d", pipeline.ID), pipeline.Ref)
		})
	}

	// Set the selected function for the pipeline list
	pipelineList.SetSelectedFunc(func(index int, _ string, _ string, _ rune) {
		// Handle selection logic here if needed
	})

	// Create a new flex container to hold the list
	flex := tview.NewFlex().
		AddItem(pipelineList, 0, 1, false)

	// Set the root of the application to the flex container
	app.SetRoot(flex, true).SetFocus(pipelineList)
}

func fetchAndShowJobs(app *tview.Application, projectID, pipelineID, pipelineName string) {
	pipelineJobs, _, err := gitlabClient.Jobs.ListPipelineJobs(projectID, toInt(pipelineID), &gitlab.ListJobsOptions{})
	if err != nil {
		fmt.Println("Error fetching jobs for project", projectID, "and pipeline", pipelineID, ":", err)
		return
	}

	jobList := tview.NewList().ShowSecondaryText(false)

	for _, job := range pipelineJobs {
		jobInfo := fmt.Sprintf("Job ID: %d \nName: %s \nStatus: %s", job.ID, job.Name, job.Status)
		jobList.AddItem(jobInfo, "", 0, nil)
	}

	jobList.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		selectedJob := pipelineJobs[index]

		jobActionModal := tview.NewModal().
			SetText(fmt.Sprintf("Select Action for Job %d", selectedJob.ID)).
			AddButtons([]string{"Logs", "Retry", "Cancel"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				switch buttonLabel {
				case "Logs":
					fetchAndDisplayJobLogs(app, projectID, strconv.Itoa(selectedJob.ID))
				case "Retry":
					retryJob(app, projectID, strconv.Itoa(selectedJob.ID))
				case "Cancel":
					newFlex := tview.NewFlex().AddItem(jobList, 0, 1, false)
					app.SetRoot(newFlex, true).SetFocus(jobList)
				}
			})

		app.SetRoot(jobActionModal, false).SetFocus(jobActionModal)
	})

	flex := tview.NewFlex().AddItem(jobList, 0, 1, false)
	app.SetRoot(flex, true).SetFocus(jobList)
}

func toInt(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

// Display logs in a modal or a new view
func fetchAndDisplayJobLogs(app *tview.Application, projectID, jobID string) {
	logsReader, _, err := gitlabClient.Jobs.GetTraceFile(projectID, toInt(jobID))
	if err != nil {
		fmt.Println("Error fetching logs:", err)
		return
	}

	logs, err := io.ReadAll(logsReader)
	if err != nil {
		fmt.Println("Error reading logs:", err)
		return
	}

	logView := tview.NewTextView().
		SetText(string(logs)).
		SetScrollable(true).
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true)

	app.SetRoot(logView, true).SetFocus(logView)
}

func retryJob(app *tview.Application, projectID, jobID string) {
	_, _, err := gitlabClient.Jobs.RetryJob(projectID, toInt(jobID))
	if err != nil {
		fmt.Println("Error retrying job:", err)
		return
	}

	fmt.Println("Job retried successfully")
}
