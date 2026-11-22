# kd completion for PowerShell 7 (pwsh), printed by `kd --completion pwsh`.
# Load it from $PROFILE with:
#     kd --completion pwsh | Out-String | Invoke-Expression
Register-ArgumentCompleter -Native -CommandName kd, kd.exe -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    # Send the words before the cursor, plus the word being typed only when it
    # starts with "-", the same request the bash, zsh and fish scripts make.
    $words = @($commandAst.CommandElements |
        Where-Object { $_.Extent.EndOffset -le $cursorPosition } |
        ForEach-Object { $_.Extent.Text })
    if ($wordToComplete -ne '' -and $words.Count -gt 1) {
        $words = @($words | Select-Object -SkipLast 1)
    }
    $request = @($words | Select-Object -Skip 1)
    if ($wordToComplete.StartsWith('-')) {
        $request += $wordToComplete
    }

    & $words[0] @request --generate-shell-completion 2>$null | ForEach-Object {
        $token, $description = $_ -split ':', 2
        if ($token -and $token.StartsWith($wordToComplete, [StringComparison]::OrdinalIgnoreCase)) {
            if (-not $description) { $description = $token }
            [System.Management.Automation.CompletionResult]::new($token, $token, 'ParameterValue', $description)
        }
    }
}
