//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package main reproduces repeated await_user_reply routing for a transferred
// sub-agent without requiring a real model provider.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session"
	sessioninmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	appName   = "await-user-reply-route-demo"
	userID    = "demo-user"
	sessionID = "demo-session"

	coordinatorName = "HealthHelperCS"
	diagnosisName   = "game_issue_diagnosis"
)

func main() {
	ctx := context.Background()
	sessionService := sessioninmemory.NewSessionService()
	diagnosis := &diagnosisAgent{name: diagnosisName}
	coordinator := &coordinatorAgent{
		name:  coordinatorName,
		child: diagnosis,
	}
	r := runner.NewRunner(
		appName,
		coordinator,
		runner.WithSessionService(sessionService),
		runner.WithAwaitUserReplyRouting(true),
	)
	defer r.Close()

	key := session.Key{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	}
	turns := []string{
		"无法玩游戏",
		"是",
		"5",
	}

	for i, input := range turns {
		fmt.Printf("\nTurn %d user: %s\n", i+1, input)
		if err := runTurn(ctx, r, input, i+1); err != nil {
			log.Fatalf("run turn %d: %v", i+1, err)
		}
		if err := printPendingRoute(ctx, sessionService, key, i+1); err != nil {
			log.Fatalf("read pending route after turn %d: %v", i+1, err)
		}
	}

	if coordinator.calls != 1 {
		log.Fatalf(
			"unexpected coordinator calls: got %d, want 1. Turn 3 was routed to the coordinator.",
			coordinator.calls,
		)
	}
	if diagnosis.calls != 3 {
		log.Fatalf(
			"unexpected diagnosis calls: got %d, want 3. Turn 3 did not resume the sub-agent.",
			diagnosis.calls,
		)
	}
	fmt.Printf(
		"\nOK: Turn 3 resumed %s directly. coordinator_calls=%d diagnosis_calls=%d\n",
		diagnosisName,
		coordinator.calls,
		diagnosis.calls,
	)
}

func runTurn(
	ctx context.Context,
	r runner.Runner,
	input string,
	turn int,
) error {
	eventCh, err := r.Run(
		ctx,
		userID,
		sessionID,
		model.NewUserMessage(input),
		agent.WithRequestID(fmt.Sprintf("await-user-reply-route-turn-%d", turn)),
	)
	if err != nil {
		return err
	}
	for evt := range eventCh {
		if evt == nil || evt.Response == nil {
			continue
		}
		if content := responseContent(evt.Response); content != "" {
			fmt.Printf("  agent=%s branch=%s content=%s\n", evt.Author, evt.Branch, content)
		}
	}
	return nil
}

func printPendingRoute(
	ctx context.Context,
	sessionService session.Service,
	key session.Key,
	turn int,
) error {
	sess, err := sessionService.GetSession(ctx, key)
	if err != nil {
		return err
	}
	route, ok, err := agent.PendingAwaitUserReplyRoute(sess)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Printf("  after turn %d pending_route=<none>\n", turn)
		return nil
	}
	fmt.Printf(
		"  after turn %d pending_route agent=%s lookup_path=%s\n",
		turn,
		route.AgentName,
		route.LookupPath,
	)
	return nil
}

func responseContent(rsp *model.Response) string {
	if rsp == nil {
		return ""
	}
	for _, choice := range rsp.Choices {
		if choice.Message.Content != "" {
			return choice.Message.Content
		}
	}
	return ""
}

type coordinatorAgent struct {
	name  string
	child *diagnosisAgent
	calls int
}

func (a *coordinatorAgent) Info() agent.Info {
	return agent.Info{Name: a.name}
}

func (a *coordinatorAgent) SubAgents() []agent.Agent {
	return []agent.Agent{a.child}
}

func (a *coordinatorAgent) FindSubAgent(name string) agent.Agent {
	if a.child != nil && a.child.Info().Name == name {
		return a.child
	}
	return nil
}

func (a *coordinatorAgent) Tools() []tool.Tool {
	return nil
}

func (a *coordinatorAgent) Run(
	ctx context.Context,
	inv *agent.Invocation,
) (<-chan *event.Event, error) {
	a.calls++
	ch := make(chan *event.Event, 4)
	go func() {
		defer close(ch)
		if strings.Contains(inv.Message.Content, "无法玩游戏") {
			childInv := inv.Clone(agent.WithInvocationAgent(a.child))
			childCh, err := a.child.Run(ctx, childInv)
			if err != nil {
				emitFinal(ctx, inv, ch, a.name, err.Error())
				return
			}
			for evt := range childCh {
				ch <- evt
			}
			return
		}
		emitFinal(
			ctx,
			inv,
			ch,
			a.name,
			"coordinator handled this turn; the pending route pointed to the root agent",
		)
	}()
	return ch, nil
}

type diagnosisAgent struct {
	name  string
	calls int
}

func (a *diagnosisAgent) Info() agent.Info {
	return agent.Info{Name: a.name}
}

func (a *diagnosisAgent) SubAgents() []agent.Agent {
	return nil
}

func (a *diagnosisAgent) FindSubAgent(string) agent.Agent {
	return nil
}

func (a *diagnosisAgent) Tools() []tool.Tool {
	return nil
}

func (a *diagnosisAgent) Run(
	ctx context.Context,
	inv *agent.Invocation,
) (<-chan *event.Event, error) {
	a.calls++
	ch := make(chan *event.Event, 1)
	go func() {
		defer close(ch)
		switch a.calls {
		case 1:
			_ = agent.MarkAwaitingUserReply(inv)
			emitFinal(ctx, inv, ch, a.name, "请确认是否当前账号")
		case 2:
			_ = agent.MarkAwaitingUserReply(inv)
			emitFinal(ctx, inv, ch, a.name, "请选择问题类型: 1 2 3 4 5")
		default:
			emitFinal(ctx, inv, ch, a.name, "sub-agent received option 5")
		}
	}()
	return ch, nil
}

func emitFinal(
	ctx context.Context,
	inv *agent.Invocation,
	ch chan<- *event.Event,
	author string,
	content string,
) {
	_ = agent.EmitEvent(
		ctx,
		inv,
		ch,
		event.NewResponseEvent(
			inv.InvocationID,
			author,
			&model.Response{
				Done: true,
				Choices: []model.Choice{{
					Index: 0,
					Message: model.Message{
						Role:    model.RoleAssistant,
						Content: content,
					},
				}},
			},
		),
	)
}
