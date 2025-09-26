# UIN 验证工作流示例

这是一个使用 trpc-agent-go 图工作流进行 UIN（用户标识号）验证的示例程序。

## 功能特性

- **智能 UIN 提取**：使用 LLM 从用户输入中智能识别和提取 UIN
- **图工作流**：基于状态图的工作流引擎，支持节点间的复杂流转
- **中断与恢复**：支持工作流中断和从检查点恢复执行
- **交互式命令行**：提供友好的命令行交互界面
- **检查点管理**：自动保存执行状态，支持工作流的暂停和恢复

## UIN 验证规则

- UIN 必须是纯数字（仅包含数字）
- UIN 长度应在 6-12 位之间
- 支持常见格式：
  - `UIN: 123456`
  - `My UIN is 123456`
  - `UIN123456`

## 安装依赖

```bash
go mod tidy
```


## 使用方法

### 启动程序

```bash
export OPENAI_API_KEY="your-api-key-here"
export OPENAI_BASE_URL="your-base-url-here" 

go run main.go
```

### 命令行参数



- `-model string`：指定使用的模型名称（默认：deepseek-chat）
- `-verbose`：启用详细输出
- `-interactive`：启用交互模式（默认：true）

### 交互式命令

程序启动后，您可以使用以下命令：

#### 1. 检查 UIN
```
check <lineageID> <input>
```
启动新的 UIN 验证工作流。

**示例：**
```
check test-001 UIN: 123456789
check user-validation My UIN is 987654321
```

#### 2. 重新检查（恢复工作流）
```
recheck <lineageID> [input]
```
从中断点恢复工作流执行。

**示例：**
```
recheck test-001 UIN: 555666777
recheck user-validation
```

#### 3. 帮助
```
help
```
显示所有可用命令的帮助信息。

#### 4. 退出
```
exit
quit
```
退出程序。

## 工作流程

1. **启动节点**：初始化工作流状态
2. **UIN 检查节点**：
   - 调用 LLM 分析用户输入
   - 如果找到有效 UIN，返回提取的数字
   - 如果未找到 UIN，中断工作流等待用户提供更多信息
3. **完成节点**：显示最终验证结果

## 状态管理

工作流使用以下状态键：

- `user_input`：用户输入的文本
- `uin_found`：是否找到有效 UIN
- `extracted_uin`：提取的 UIN 数字
- `last_node`：最后执行的节点
- `is_user_trigger`：是否为用户触发的恢复

## 检查点功能

- **自动保存**：工作流执行过程中自动创建检查点
- **中断处理**：当 LLM 无法识别 UIN 时，工作流会中断并保存状态
- **恢复执行**：使用 `recheck` 命令可以从中断点继续执行
- **状态持久化**：所有状态信息都会持久化保存

## 示例会话

```

🎯 UIN Validation Interactive Mode
Available commands: check, recheck, list, tree, history, latest, delete, status, help, exit
Type 'help' for detailed command descriptions.


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
uin-checker> check uin test
2025-09-26T10:41:34+08:00       INFO    checkUin/main.go:403    Starting workflow execution: lineage_id=uin, namespace=, user_input=test

🚀 Running workflow normally (lineage: uin)...
⚡ Executing: start
2025-09-26T10:41:34+08:00       INFO    graph/executor.go:1732  Starting UIN validation workflow
2025-09-26T10:41:34+08:00       INFO    graph/executor.go:1732  Executing UIN validation node
🔄 UIN validation node is running, is_resuming: false
🔄 UIN validation node is running with input: test
⚡ Executing: check_uin
🔍 LLM response: EMPTY
🔄 UIN validation node is waiting for user input
🔄 Interrupting UIN validation node
⚠️  Input validation required. Please provide a valid UIN by [recheck lineage [input]]
💾 Execution interrupted, checkpoint saved
uin-checker> recheck uin 200
2025-09-26T10:41:39+08:00       INFO    graph/executor.go:1732  Executing UIN validation node
🔄 UIN validation node is running, is_resuming: true
🔄 UIN validation node is waiting for user input
🔄 Interrupting UIN validation node
🔄 UIN validation node is running with input: 200
🔍 LLM response: EMPTY
🔄 UIN validation node is waiting for user input
⚠️  Workflow interrupted again
   Use 'recheck uin [uin desc] to continue
uin-checker>   
```

