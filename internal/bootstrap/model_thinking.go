package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/llm"
)

// ParseThinkingLevel 把配置字符串转 agentcore.ThinkingLevel。
// "" 合法（= 不覆盖/继承）；其余须是 off/low/medium/high/xhigh/max 之一，
// 否则返回 error（启动时降级当空并 warn，运行时把 error 回显给用户）。
func ParseThinkingLevel(s string) (agentcore.ThinkingLevel, error) {
	lv := agentcore.NormalizeThinkingLevel(agentcore.ThinkingLevel(s))
	switch lv {
	case "", agentcore.ThinkingOff, agentcore.ThinkingLow, agentcore.ThinkingMedium,
		agentcore.ThinkingHigh, agentcore.ThinkingXHigh, agentcore.ThinkingMax:
		return lv, nil
	default:
		return "", fmt.Errorf("无效推理强度 %q（可选：off/low/medium/high/xhigh/max）", s)
	}
}

func ResolveThinkingForModel(model agentcore.ChatModel, level agentcore.ThinkingLevel) (agentcore.ThinkingLevel, bool) {
	level = agentcore.NormalizeThinkingLevel(level)
	// 对不支持 thinking 的普通 chat 模型，显式 off 不是 no-op，而是非法参数。
	if cp, ok := model.(llm.CapabilityProvider); ok && cp.Capabilities().Thinking.Supported == llm.SupportNo {
		return agentcore.ThinkingAuto, level == agentcore.ThinkingAuto
	}
	return llm.ThinkingPolicyFor(model).Resolve(level)
}

func AvailableThinkingForModel(model agentcore.ChatModel) []agentcore.ThinkingLevel {
	if cp, ok := model.(llm.CapabilityProvider); ok && cp.Capabilities().Thinking.Supported == llm.SupportNo {
		return []agentcore.ThinkingLevel{agentcore.ThinkingAuto}
	}
	return llm.ThinkingPolicyFor(model).Available
}

// Bind the preference to the role, since different roles can share the same
// underlying provider client and each may have a different reasoning policy.
func (ms *ModelSet) newRoleModel(role, provider, name string, target agentcore.ChatModel, schema *bool) *SwappableModel {
	if _, err := ParseThinkingLevel(ms.config.ResolveReasoningEffort(role)); err != nil {
		slog.Warn("忽略无效推理强度配置", "module", "config", "role", role, "err", err)
	}
	m := NewSwappableModel(provider, name, target, schema)
	m.role, m.set = role, ms
	return m
}

// SetReasoningEffort updates an existing binding after configuration persistence succeeds.
func (ms *ModelSet) SetReasoningEffort(role, effort string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if role == "" || role == "default" {
		ms.config.ReasoningEffort = effort
		return
	}
	if ms.config.Roles == nil {
		ms.config.Roles = make(map[string]RoleConfig)
	}
	rc := ms.config.Roles[role]
	rc.ReasoningEffort = effort
	ms.config.Roles[role] = rc
}

// Snapshot the selected model and the effective preference under the same
// ModelSet lock used by ApplyPrepared. Retries retain this request's intent.
func (m *SwappableModel) requestTarget() modelTarget {
	var level agentcore.ThinkingLevel
	if m.set != nil {
		m.set.mu.RLock()
		defer m.set.mu.RUnlock()
		level, _ = ParseThinkingLevel(m.set.config.ResolveReasoningEffort(m.role))
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return modelTarget{
		provider: m.provider, name: m.name, model: m.SwappableModel.Current(),
		jsonSchema: cloneBoolPtr(m.jsonSchema), reasoning: level,
	}
}

func (target modelTarget) callOptions(opts []agentcore.CallOption) []agentcore.CallOption {
	if target.reasoning == agentcore.ThinkingAuto {
		return opts
	}
	resolved, _ := ResolveThinkingForModel(target.model, target.reasoning)
	if resolved == agentcore.ThinkingAuto {
		return opts
	}
	// Prepending also lets an explicit WithThinking(Auto) clear a configured
	// default. Resolving opts first would conflate that choice with no option.
	return append([]agentcore.CallOption{agentcore.WithThinking(resolved)}, opts...)
}

func (m *SwappableModel) Generate(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	target := m.requestTarget()
	if target.model == nil {
		return nil, agentcore.ErrNoModel
	}
	return target.model.Generate(ctx, messages, tools, target.callOptions(opts)...)
}

func (m *SwappableModel) GenerateStream(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (<-chan agentcore.StreamEvent, error) {
	target := m.requestTarget()
	if target.model == nil {
		return nil, agentcore.ErrNoModel
	}
	return target.model.GenerateStream(ctx, messages, tools, target.callOptions(opts)...)
}
