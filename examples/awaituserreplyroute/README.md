# Await User Reply Route Demo

This example reproduces repeated `await_user_reply` routing after a transfer to
a sub-agent.

It does not call a real model. The coordinator and sub-agent are local mock
agents so the routing behavior is deterministic.

## Scenario

1. User says `无法玩游戏`.
2. `HealthHelperCS` simulates a transfer to `game_issue_diagnosis`.
3. `game_issue_diagnosis` marks `await_user_reply`.
4. User says `是`.
5. Runner resumes directly into `game_issue_diagnosis`.
6. `game_issue_diagnosis` marks `await_user_reply` again and asks the user to
   choose `1 2 3 4 5`.
7. User says `5`.
8. Runner should resume directly into `game_issue_diagnosis` again.

## Run

```bash
cd examples
go run ./awaituserreplyroute
```

With the fix, Turn 2 keeps the full pending route:

```text
after turn 2 pending_route agent=game_issue_diagnosis lookup_path=HealthHelperCS/game_issue_diagnosis
OK: Turn 3 resumed game_issue_diagnosis directly.
```

Without the fix, Turn 2 drops the sub-agent segment:

```text
after turn 2 pending_route agent=HealthHelperCS lookup_path=HealthHelperCS
```

Then Turn 3 is routed to `HealthHelperCS` instead of
`game_issue_diagnosis`, matching the user-reported behavior.
