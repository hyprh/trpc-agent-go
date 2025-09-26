//
// Copyright (C) 2024 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// Licensed under the BSD 3-Clause License (the "License"); you may not use this file except
// in compliance with the License. You may obtain a copy of the License at
//
// https://opensource.org/licenses/BSD-3-Clause
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is
// distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and limitations under the License.

// Package main demonstrates UIN (User Identification Number) validation using trpc-agent-go graph workflow.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"reflect"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver (install with: go get github.com/mattn/go-sqlite3)

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/graphagent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/graph"
	checkpointinmemory "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/log"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	sessioninmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

const (
	// Default configuration values
	defaultModelName     = "deepseek-chat"
	defaultUserID        = "uin-checker-user"
	defaultAppName       = "uin-validation-workflow"
	defaultLineagePrefix = "uin-check-demo"
	defaultNamespace     = ""

	// State keys for workflow state management
	stateKeyUserInput     = "user_input"
	stateKeyUINFound      = "uin_found"
	stateKeyExtractedUIN  = "extracted_uin"
	stateKeyLastNode      = "last_node"
	stateKeyIsUserTrigger = "is_user_trigger"

	// Node names for the workflow graph
	nodeStart    = "start"
	nodeCheckUIN = "check_uin"
	nodeFinalize = "finalize"

	// Command constants for interactive mode
	cmdCheck   = "check"
	cmdRecheck = "recheck"
	cmdHelp    = "help"
	cmdExit    = "exit"
	cmdQuit    = "quit"

	// UIN validation prompt for LLM
	uinValidationPrompt = `You are a UIN (User Identification Number) validation expert. 
Analyze the provided text and determine if it contains a valid UIN.

UIN Rules:
- UIN must be a numeric value (digits only)
- UIN should be between 6-12 digits long
- Common formats: "UIN: 123456", "My UIN is 123456", "UIN123456"

Instructions:
- If you find a valid UIN, return only the numeric digits
- If no valid UIN is found, return exactly "EMPTY"
- Do not include any explanations or additional text

Examples:
- Input: "UIN: 123456" → Output: "123456"
- Input: "My UIN is 987654321" → Output: "987654321"  
- Input: "Hello world" → Output: "EMPTY"`

	// UI messages for better user experience
	msgUINPrompt         = "Enter UIN (format: 'UIN: 123456' or 'My UIN is 123456'): "
	msgUINNotFound       = "⚠️  No valid UIN found in your input. Please try again."
	msgUINFound          = "✅ Valid UIN found: %s"
	msgInterruptDetected = "⚠️  Input validation required. Please provide a valid UIN by [recheck lineage [input]]"
)

var (
	modelName       = flag.String("model", defaultModelName, "Name of the model to use")
	verbose         = flag.Bool("verbose", false, "Enable verbose output")
	interactiveMode = flag.Bool("interactive", true, "Enable interactive command-line mode")
)

// uinWorkflow represents the UIN validation workflow with all necessary components.
type uinWorkflow struct {
	modelName        string
	modelInstance    model.Model
	verbose          bool
	logger           log.Logger
	runner           runner.Runner
	graphAgent       *graphagent.GraphAgent
	saver            graph.CheckpointSaver
	manager          *graph.CheckpointManager
	currentLineageID string
	currentNamespace string
	userID           string
	sessionID        string
}

func main() {
	flag.Parse()

	workflow := &uinWorkflow{
		modelName: *modelName,
		logger:    log.Default,
		verbose:   *verbose,
		userID:    defaultUserID,
		sessionID: "session-" + generateLineageID(),
	}

	if err := workflow.setup(); err != nil {
		fmt.Printf("❌ Setup failed: %s\n", err)
		os.Exit(1)
	}

	if err := workflow.run(); err != nil {
		fmt.Printf("❌ Execution failed: %s\n", err)
		os.Exit(1)
	}
}

// run executes the main workflow logic.
func (w *uinWorkflow) run() error {
	ctx := context.Background()

	if *interactiveMode {
		return w.startInteractiveMode(ctx)
	}

	// Non-interactive mode: run once with command line args
	lineageID := generateLineageID()
	fmt.Print(msgUINPrompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		userInput := strings.TrimSpace(scanner.Text())
		return w.runWorkflow(ctx, lineageID, userInput)
	}
	return fmt.Errorf("failed to read user input")
}

// setup initializes all components needed for the workflow.
func (w *uinWorkflow) setup() error {
	// Create model instance
	w.modelInstance = openai.New(w.modelName)

	// Create graph
	compiledGraph, err := w.createUINValidationGraph()
	if err != nil {
		return fmt.Errorf("failed to create graph: %w", err)
	}

	// Create checkpoint saver
	w.saver = checkpointinmemory.NewSaver()

	// Create checkpoint manager
	w.manager = graph.NewCheckpointManager(w.saver)

	// Create GraphAgent
	w.graphAgent, err = graphagent.New("uin-validator", compiledGraph,
		graphagent.WithDescription("UIN validation workflow"),
		graphagent.WithCheckpointSaver(w.saver),
	)
	if err != nil {
		return fmt.Errorf("failed to create graph agent: %w", err)
	}

	// Create Runner
	w.runner = runner.NewRunner(defaultAppName, w.graphAgent,
		runner.WithSessionService(sessioninmemory.NewSessionService()),
	)

	return nil
}

// createUINValidationGraph creates the workflow graph for UIN validation.
func (w *uinWorkflow) createUINValidationGraph() (*graph.Graph, error) {
	// Define state schema
	stateSchema := graph.NewStateSchema()
	stateSchema.AddField(stateKeyUINFound, graph.StateField{
		Type:    reflect.TypeOf(false),
		Reducer: graph.DefaultReducer,
		Default: func() any { return false },
	})
	stateSchema.AddField(stateKeyExtractedUIN, graph.StateField{
		Type:    reflect.TypeOf(""),
		Reducer: graph.DefaultReducer,
		Default: func() any { return "" },
	})
	stateSchema.AddField(stateKeyLastNode, graph.StateField{
		Type:    reflect.TypeOf(""),
		Reducer: graph.DefaultReducer,
		Default: func() any { return "" },
	})
	stateSchema.AddField(stateKeyUserInput, graph.StateField{
		Type:    reflect.TypeOf(""),
		Reducer: graph.DefaultReducer,
		Default: func() any { return "" },
	})

	// Create LLM agent for UIN validation
	agent := llmagent.New("uin-validator",
		llmagent.WithModel(w.modelInstance),
		llmagent.WithGenerationConfig(
			model.GenerationConfig{
				Stream: false,
			},
		),
		llmagent.WithInstruction(uinValidationPrompt),
	)

	// Create graph builder
	builder := graph.NewStateGraph(stateSchema)

	// Add nodes
	builder.AddNode(nodeStart, func(ctx context.Context, state graph.State) (any, error) {
		w.logger.Infof("Starting UIN validation workflow")
		return graph.State{
			stateKeyLastNode: nodeStart,
		}, nil
	})

	builder.AddNode(nodeCheckUIN, w.createUINValidationNode(agent))

	builder.AddNode(nodeFinalize, func(ctx context.Context, state graph.State) (any, error) {
		w.logger.Infof("Finalizing UIN validation workflow")

		if uinFound := getBool(state, stateKeyUINFound); uinFound {
			if extractedUIN, ok := state[stateKeyExtractedUIN].(string); ok {
				fmt.Printf(msgUINFound+"\n", extractedUIN)
			}
		} else {
			fmt.Printf(msgUINNotFound + "\n")
		}

		return graph.State{
			stateKeyLastNode: nodeFinalize,
		}, nil
	})

	// Add edges
	builder.SetEntryPoint(nodeStart)
	builder.SetFinishPoint(nodeFinalize)
	builder.AddEdge(nodeStart, nodeCheckUIN)
	builder.AddEdge(nodeCheckUIN, nodeFinalize)
	return builder.Compile()
}

// createUINValidationNode creates the UIN validation node function.
func (w *uinWorkflow) createUINValidationNode(llmAgent agent.Agent) func(context.Context, graph.State) (any, error) {
	return func(ctx context.Context, state graph.State) (any, error) {
		w.logger.Infof("Executing UIN validation node")

		userInput := state[stateKeyUserInput].(string)
		isUserTrigger := graph.ResumeValueOrDefault(ctx, state, stateKeyIsUserTrigger, false)
		isInterrupted := false
		fmt.Println("🔄 UIN validation node is running, is_resuming:", isUserTrigger)

		runner := runner.NewRunner(defaultAppName, llmAgent,
			runner.WithSessionService(sessioninmemory.NewSessionService()),
		)

		for {
			if !isUserTrigger {
				fmt.Printf("🔄 UIN validation node is running with input: %s\n", userInput)
				// Call LLM agent
				eventChan, err := runner.Run(ctx, "user", "session", model.NewUserMessage(userInput))
				if err != nil {
					fmt.Println("❌ LLM invocation failed")
					return nil, fmt.Errorf("LLM invocation failed: %w", err)
				}

				// Extract response content
				output := w.extractResponseContent(eventChan)
				fmt.Printf("🔍 LLM response: %s\n", output)
				if !strings.Contains(strings.ToLower(output), "empty") {
					fmt.Printf("✅ UIN found: %s\n", output)
					return graph.State{
						stateKeyExtractedUIN: output,
						stateKeyUINFound:     true,
					}, nil
				}
			}

			fmt.Println("🔄 UIN validation node is waiting for user input")
			if isInterrupted {
				return nil, graph.NewInterruptError(nil)
			}

			fmt.Printf("🔄 Interrupting UIN validation node\n")
			isInterrupted = true
			isUserTrigger = false
			resumeValue, err := graph.Interrupt(ctx, state, nodeCheckUIN, nil)
			if err != nil {
				return nil, err
			}
			userInput = resumeValue.(string)
		}
	}
}

// extractResponseContent extracts content from LLM response events.
func (w *uinWorkflow) extractResponseContent(eventChan <-chan *event.Event) string {
	var content strings.Builder
	for e := range eventChan {
		if e.Error != nil {
			w.logger.Errorf("Event error: %s", e.Error.Message)
			continue
		}
		if len(e.Choices) > 0 {
			choice := e.Choices[0]
			if choice.Message.Content != "" {
				content.WriteString(choice.Message.Content)
			}
		}
	}
	return strings.TrimSpace(content.String())
}

// startInteractiveMode starts the interactive command-line interface.
func (w *uinWorkflow) startInteractiveMode(ctx context.Context) error {
	fmt.Printf("\n🎯 UIN Validation Interactive Mode\n")
	fmt.Printf("Available commands: check, recheck, list, tree, history, latest, delete, status, help, exit\n")
	fmt.Printf("Type 'help' for detailed command descriptions.\n\n")

	scanner := bufio.NewScanner(os.Stdin)
	w.showHelp()
	for {
		fmt.Print("uin-checker> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		command := parts[0]
		args := parts[1:]

		switch command {
		case cmdCheck:
			if err := w.handleCheckCommand(ctx, args); err != nil {
				fmt.Printf("❌ Error: %s\n", err)
			}
		case cmdRecheck:
			if err := w.handleRecheckCommand(ctx, args); err != nil {
				fmt.Printf("❌ Error: %s\n", err)
			}
		case cmdHelp:
			w.showHelp()
		case cmdExit, cmdQuit:
			fmt.Println("👋 Goodbye!")
			return nil
		default:
			fmt.Printf("❌ Unknown command: %s. Type 'help' for available commands.\n", command)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("input error: %w", err)
	}
	return nil
}

// handleCheckCommand handles the check command.
func (w *uinWorkflow) handleCheckCommand(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: check <lineageID> <input>")
	}

	lineageID := args[0]
	userInput := strings.Join(args[1:], " ")

	if lineageID == "" {
		return fmt.Errorf("lineageID is required")
	}

	if userInput == "" {
		return fmt.Errorf("user input is required")
	}

	return w.runWorkflow(ctx, lineageID, userInput)
}

// handleRecheckCommand handles the recheck command.
func (w *uinWorkflow) handleRecheckCommand(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("lineage ID required for recheck")
	}

	lineageID := args[0]
	var userInput string
	if len(args) > 1 {
		userInput = strings.Join(args[1:], " ")
	}

	return w.resumeWorkflow(ctx, lineageID, userInput)
}

// runWorkflow executes the workflow with the given lineage ID.
func (w *uinWorkflow) runWorkflow(ctx context.Context, lineageID string, userInput string) error {
	w.currentLineageID = lineageID
	w.currentNamespace = defaultNamespace

	w.logger.Infof("Starting workflow execution: lineage_id=%s, namespace=%s, user_input=%s", lineageID, w.currentNamespace, userInput)

	fmt.Printf("\n🚀 Running workflow normally (lineage: %s)...\n", lineageID)

	// Create initial message.
	message := model.NewUserMessage(userInput)

	config := graph.NewCheckpointConfig(lineageID).WithNamespace(w.currentNamespace)
	if w.verbose {
		w.logger.Debugf("Created checkpoint configuration: %+v", config.ToMap())
	}

	// Run the workflow through the runner.
	events, err := w.runner.Run(
		ctx,
		w.userID,
		w.sessionID,
		message,
		agent.WithRuntimeState(graph.State{
			graph.CfgKeyLineageID:    lineageID,
			graph.CfgKeyCheckpointNS: w.currentNamespace,
			stateKeyUserInput:        userInput,
		}),
	)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Process events and track execution.
	count := 0
	interrupted := false
	for event := range events {
		if event.Error != nil {
			fmt.Printf("❌ Error: %s\n", event.Error.Message)
			continue
		}

		// Show node execution progress for interrupt workflows.
		if event.Object == graph.ObjectTypeGraphNodeStart {
			fmt.Printf("⚡ Executing: %s\n", event.Author)
		}
		count++
	}

	// Check if an interrupt checkpoint was created by examining actual checkpoints.
	if w.manager != nil {
		config := graph.NewCheckpointConfig(lineageID).WithNamespace(w.currentNamespace)
		checkpoints, err := w.manager.ListCheckpoints(ctx, config.ToMap(), nil)
		if err == nil {
			// Look for interrupt checkpoint.
			for _, cp := range checkpoints {
				if cp.Metadata.Source == "interrupt" {
					interrupted = true
					fmt.Printf(msgInterruptDetected + "\n")
					break
				}
			}
		}
	}

	if interrupted {
		fmt.Printf("💾 Execution interrupted, checkpoint saved\n")
	}
	return nil
}

// resumeWorkflow resumes execution from a checkpoint.
func (w *uinWorkflow) resumeWorkflow(ctx context.Context, lineageID, userInput string) error {
	w.currentLineageID = lineageID
	w.currentNamespace = defaultNamespace

	// Check if the lineage exists before attempting resume.
	config := graph.NewCheckpointConfig(lineageID).WithNamespace(w.currentNamespace)
	checkpoints, err := w.manager.ListCheckpoints(ctx, config.ToMap(), nil)
	if err != nil {
		return fmt.Errorf("failed to check lineage existence: %w", err)
	}
	if len(checkpoints) == 0 {
		return fmt.Errorf("no checkpoints found for lineage: %s", lineageID)
	}

	if userInput == "" {
		// Prompt for user input if not provided.
		fmt.Print("Enter approval decision (yes/no): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			userInput = strings.TrimSpace(scanner.Text())
		}
	}

	if userInput == "" {
		return fmt.Errorf("user input required for resume")
	}

	// Create resume command. Only set the resume value for the specific interrupt
	// that is currently active. We need to check which interrupt is active.
	cmd := &graph.Command{
		ResumeMap: make(map[string]any),
	}

	// Check which interrupt is currently active by looking at the latest checkpoint.
	// This leverages the ResumeMap design - we only set the resume value for the
	// specific interrupt that's currently active, not future ones.
	// First, list all checkpoints to find the interrupted one
	cfg := graph.CreateCheckpointConfig(lineageID, "", w.currentNamespace)
	filter := &graph.CheckpointFilter{Limit: 10}
	checkpoints, listErr := w.manager.ListCheckpoints(ctx, cfg, filter)
	if listErr != nil {
		return fmt.Errorf("failed to list checkpoints: %w", listErr)
	}

	// Find the latest interrupted checkpoint
	var latest *graph.CheckpointTuple
	for _, cp := range checkpoints {
		if cp.Checkpoint.IsInterrupted() {
			latest = cp
			break
		}
	}

	// Fallback to using Latest() if no interrupted checkpoint found in list
	if latest == nil {
		latest, err = w.manager.Latest(ctx, lineageID, w.currentNamespace)
	}
	if err != nil && latest == nil {
		if w.verbose {
			w.logger.Errorf("Failed to get latest checkpoint: %v", err)
		}
		return fmt.Errorf("failed to get latest checkpoint: %w", err)
	}
	if latest == nil {
		return fmt.Errorf("no checkpoints found for lineage: %s", lineageID)
	}
	if latest != nil && !latest.Checkpoint.IsInterrupted() {
		if w.verbose {
			w.logger.Infof("Latest checkpoint is not interrupted, ID: %s", latest.Checkpoint.ID)
		}
		return fmt.Errorf("no active interrupt found for lineage: %s (latest checkpoint is not interrupted)", lineageID)
	}

	// Use the TaskID from the interrupt state as the key.
	// This automatically handles any interrupt without needing to know specific names.
	taskID := latest.Checkpoint.InterruptState.TaskID
	cmd.ResumeMap[taskID] = userInput
	cmd.ResumeMap[stateKeyIsUserTrigger] = true

	if w.verbose {
		w.logger.Infof("Setting resume value for TaskID '%s' to '%s'", taskID, userInput)
	}

	message := model.NewUserMessage("resume")
	runtimeState := graph.State{
		graph.StateKeyCommand:    cmd,
		graph.CfgKeyLineageID:    lineageID,
		graph.CfgKeyCheckpointNS: w.currentNamespace,
	}

	events, err := w.runner.Run(
		ctx,
		w.userID,
		w.sessionID,
		message,
		agent.WithRuntimeState(runtimeState),
	)
	if err != nil {
		return fmt.Errorf("resume failed: %w", err)
	}

	// Process events.
	count := 0
	var lastNode string
	interrupted := false
	for event := range events {
		if event.Error != nil {
			fmt.Printf("❌ Error: %s\n", event.Error.Message)
			continue
		}

		// Track node execution for verbose output.
		if w.verbose && event.Author != "" && event.Object == "graph.node.start" {
			fmt.Printf("⚡ Executing: %s\n", event.Author)
		}

		// Track execution.
		if event.Author == graph.AuthorGraphNode {
			if event.StateDelta != nil {
				if metadata, ok := event.StateDelta[graph.MetadataKeyNode]; ok {
					var nodeMetadata graph.NodeExecutionMetadata
					if err := json.Unmarshal(metadata, &nodeMetadata); err == nil {
						if nodeMetadata.NodeID != "" {
							lastNode = nodeMetadata.NodeID
						}
					}
				}
			}
		}
		count++
	}

	// Check if the workflow completed or was interrupted again.
	workflowCompleted := false
	if w.manager != nil {
		config := graph.NewCheckpointConfig(lineageID).WithNamespace(w.currentNamespace)
		checkpoints, err := w.manager.ListCheckpoints(ctx, config.ToMap(), nil)
		if err == nil && len(checkpoints) > 0 {
			// Check the latest checkpoint
			latest := checkpoints[0]

			// If the latest checkpoint is a regular checkpoint (not interrupt),
			// and it's from a step after the resume, the workflow likely completed or progressed
			if latest.Metadata.Source != "interrupt" {
				// Check if we reached the final node
				if state := w.extractRootState(latest.Checkpoint); state != nil {
					if lastNodeInState, ok := state[stateKeyLastNode].(string); ok && lastNodeInState == nodeFinalize {
						workflowCompleted = true
					}
				}
			} else if latest.Checkpoint.IsInterrupted() {
				// There's a new interrupt
				interrupted = true
			}
		}
	}

	if workflowCompleted {
		fmt.Printf("✅ Workflow completed successfully!\n")
		fmt.Printf("   Total events: %d\n", count)
		if lastNode != "" {
			fmt.Printf("   Final node: %s\n", lastNode)
		}
	} else if interrupted {
		fmt.Printf("⚠️  Workflow interrupted again\n")
		fmt.Printf("   Use 'recheck %s [uin desc] to continue\n", lineageID)
	} else {
		fmt.Printf("✅ recheck completed (%d events)\n", count)
		if lastNode != "" {
			fmt.Printf("   Last node: %s\n", lastNode)
		}
	}
	return nil
}

// showHelp displays help information.
func (w *uinWorkflow) showHelp() {
	fmt.Printf(`
🎯 UIN Validation Workflow Commands

Available Commands:
  check <lineage> [input]           - Start new UIN validation workflow
                           If input not provided, will prompt for it
  recheck <lineage> [input] - Resume interrupted workflow
                           If input not provided, will prompt for it

  help                    - Show this help message
  exit/quit              - Exit the program

Examples:
  check uin test
  recheck uin 200
UIN Format:
  - Must be 6-12 digits long
  - Common formats: "UIN: 123456", "My UIN is 123456", "UIN123456"
`)
}

// extractRootState extracts the root state from a checkpoint.
func (w *uinWorkflow) extractRootState(checkpoint *graph.Checkpoint) map[string]any {
	if checkpoint.ChannelValues == nil {
		return nil
	}

	state := make(map[string]any)
	maps.Copy(state, checkpoint.ChannelValues)

	return state
}

// getBool safely extracts a boolean value from state.
func getBool(s graph.State, key string) bool {
	if val, ok := s[key]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return false
}

// generateLineageID generates a unique lineage ID.
func generateLineageID() string {
	return fmt.Sprintf("%s-%s", defaultLineagePrefix, time.Now().Format("20060102-150405"))
}
