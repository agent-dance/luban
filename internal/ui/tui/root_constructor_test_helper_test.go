package tui

func NewRootComponent(state *AppState, onSubmit func(string), slashCommands []SlashCommandEntry) *RootComponent {
	return newRootComponent(state, onSubmit, nil, slashCommands)
}
