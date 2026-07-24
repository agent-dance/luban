package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

// MCPPromptRunner is the command package's narrow dependency on the MCP
// service layer. *services/mcp.Manager satisfies it.
type MCPPromptRunner interface {
	PromptCommandDescriptors(context.Context) ([]svcmcp.PromptCommandDescriptor, error)
	ExecutePromptCommand(context.Context, svcmcp.PromptCommandDescriptor, string) ([]types.Message, error)
}

// RegisterMCPPromptCommands registers connected MCP prompts as slash commands.
// It is deliberately separate from /mcp management UI registration so callers
// can wire dynamic prompts without importing the command registry into services.
func RegisterMCPPromptCommands(ctx context.Context, registry *Registry, runner MCPPromptRunner) error {
	if registry == nil {
		return errors.New(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeMCPPromptRegistryMissing))
	}
	commands, err := NewMCPPromptCommands(ctx, runner)
	if err != nil {
		return err
	}
	for _, command := range commands {
		registry.Register(command)
	}
	return nil
}

// NewMCPPromptCommands returns command adapters for all connected MCP prompts.
func NewMCPPromptCommands(ctx context.Context, runner MCPPromptRunner) ([]Command, error) {
	if runner == nil {
		return nil, errors.New(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeMCPPromptRunnerMissing))
	}
	descriptors, err := runner.PromptCommandDescriptors(ctx)
	if err != nil {
		return nil, err
	}
	return MCPPromptCommandsFromDescriptors(descriptors, runner), nil
}

// MCPPromptCommandsFromDescriptors wraps known prompt descriptors. Tests and
// notification refresh paths can use this when they already have fresh catalogues.
func MCPPromptCommandsFromDescriptors(descriptors []svcmcp.PromptCommandDescriptor, runner interface {
	ExecutePromptCommand(context.Context, svcmcp.PromptCommandDescriptor, string) ([]types.Message, error)
}) []Command {
	commands := make([]Command, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if strings.TrimSpace(descriptor.Name) == "" {
			continue
		}
		commands = append(commands, &MCPPromptCommand{Descriptor: descriptor, Runner: runner})
	}
	return commands
}

// MCPPromptCommand is a slash command backed by prompts/get.
type MCPPromptCommand struct {
	Descriptor svcmcp.PromptCommandDescriptor
	Runner     interface {
		ExecutePromptCommand(context.Context, svcmcp.PromptCommandDescriptor, string) ([]types.Message, error)
	}
}

func (c *MCPPromptCommand) Name() string { return c.Descriptor.Name }

func (c *MCPPromptCommand) Aliases() []string { return nil }

func (c *MCPPromptCommand) Description() string {
	if strings.TrimSpace(c.Descriptor.Description) != "" {
		return c.Descriptor.Description
	}
	lang := i18n.DetectOrLoadLanguage()
	return i18n.Format(lang, i18n.KeyCommandPresentationMCPPromptDescription, c.Descriptor.PromptName, c.Descriptor.ServerName)
}

// PresentationContract keeps dynamically discovered MCP prompts on the same
// typed display path as the built-in MCP manager command.
func (c *MCPPromptCommand) PresentationContract() CommandPresentationContract {
	name := "mcp-prompt"
	target := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeMCPPromptTarget)
	if c != nil {
		name = c.Name()
		target = c.UserFacingName()
	}
	return CommandPresentationContract{
		Command: name, Family: CommandFamilyIntegration, Display: CommandDisplayReceipt, Risk: CommandRiskMedium,
		DefaultAction: "run-prompt", DefaultTarget: target,
		CompletedNextAction:     i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCommandPresentationMCPPromptSuccess),
		FailedNextAction:        i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCommandPresentationMCPPromptFailure),
		CompletedNextActionKey:  i18n.KeyCommandPresentationMCPPromptSuccess,
		FailedNextActionKey:     i18n.KeyCommandPresentationMCPPromptFailure,
		TerminalOutcomeReliable: true,
	}
}

func (c *MCPPromptCommand) Execute(ctx *Context, args string) error {
	if c == nil || c.Runner == nil {
		return errors.New(i18n.Text(commandContextLanguage(ctx), i18n.KeyRuntimeMCPPromptRunnerMissing))
	}
	if ctx == nil || ctx.QueryLoop == nil {
		return errors.New(i18n.Text(commandContextLanguage(ctx), i18n.KeyRuntimeMCPPromptQueryLoopMissing))
	}
	messages, err := c.Runner.ExecutePromptCommand(context.Background(), c.Descriptor, args)
	if err != nil {
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyRuntimeMCPPromptRunFailed, c.Descriptor.Name, err))
	}
	existing := ctx.QueryLoop.Messages()
	next := make([]types.Message, 0, len(existing)+len(messages))
	next = append(next, existing...)
	next = append(next, messages...)
	if updater, ok := ctx.QueryLoop.(SameSessionMessageUpdater); ok {
		updater.SetMessagesPreservingToolUseLedger(next)
	} else {
		// Persistent engine adapters preserve sidecar metadata while saving the
		// rewritten transcript. Legacy implementations retain replacement
		// behavior for source compatibility.
		ctx.QueryLoop.SetMessages(next)
	}
	if ctx.OnEvent != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyMCPPromptRan, c.UserFacingName()))
	}
	return nil
}

func commandContextLanguage(ctx *Context) i18n.Language {
	if ctx != nil {
		return ctx.Language
	}
	return i18n.DetectOrLoadLanguage()
}

// ArgumentHint returns the progressive prompt argument hint used by typeahead.
func (c *MCPPromptCommand) ArgumentHint() string {
	return c.Descriptor.ArgumentHint
}

func (c *MCPPromptCommand) ArgumentNames() []string {
	return append([]string(nil), c.Descriptor.ArgumentNames...)
}

func (c *MCPPromptCommand) RequiredArgumentNames() []string {
	return append([]string(nil), c.Descriptor.RequiredArguments...)
}

func (c *MCPPromptCommand) IsMCP() bool { return true }

func (c *MCPPromptCommand) UserFacingName() string {
	return fmt.Sprintf("%s:%s (MCP)", c.Descriptor.ServerName, c.Descriptor.PromptName)
}
