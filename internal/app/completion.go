package app

import (
	"fmt"
)

const powershellCompletion = `Register-ArgumentCompleter -Native -CommandName phpvm -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $commands = @('use','install','ls','list','ls-remote','current','which','uninstall','prune','verify','repair','doctor','clean','exec','matrix','alias','sync','ini','profile','ext','logs','cache','self-update','completion','laragon','version','help')
    $parts = @($commandAst.CommandElements | ForEach-Object { $_.Extent.Text })
    if ($parts.Count -le 2) {
        $commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }
    $subcommands = @{
        logs=@('path','show','tail','open','clear','doctor'); cache=@('dir','clear')
        ini=@('path','show','diff','reset','get','set'); profile=@('ls','create','set','use')
        ext=@('ls','enable','disable'); alias=@('ls','set','remove')
        laragon=@('detect','link','unlink'); completion=@('powershell')
    }
    $command = $parts[1]
    if ($subcommands.ContainsKey($command) -and $parts.Count -le 3) {
        $subcommands[$command] | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }
    if ($command -in @('use','uninstall','verify','repair','which')) {
        (& phpvm ls 2>$null) -replace '^\*?\s+', '' | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
    }
}
`

func (a *App) completion(args []string) error {
	if len(args) != 1 || args[0] != "powershell" {
		return fmt.Errorf("usage: phpvm completion powershell")
	}
	fmt.Fprint(a.Out, powershellCompletion)
	return nil
}
