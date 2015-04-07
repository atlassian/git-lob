# Coding Conventions #

1. **gofmt**
   
   Always run gofmt after saving to format your code to standard Go 
   convention. It is recommended that you use a plugin to your
   text editor that does this automatically, e.g. GoSublime

2. **Command line entry points**
   
  When implementing a new command:
  1. Implement entry point in cmd/cmd[*command*].go
  2. The main entry point should be a function called cmd.Command()
  3. Use this to validate input & parse arguments, then call an implementation function called core.Command(), implemented in core/[*command*].go, passing in the parsed arguments
  4. Implement a function called cmd.CommandHelp() to display the --help output
  5. Modify main.go and add a new command entry in the switch

3. **Output messages and logging**

   There are 2 sets of output - the console and optionally a log file, if
   configured by the user. In addition there are 2 run-time options which 
   control how much is output, the '--quiet' and '--verbose' options.

   In order to work with these most easily, you should never output directly
   to any console streams. You should also never need to check the verbose
   and quiet options yourself. Instead, use the following utilities.

   * **For simultaneous log & console output:**

     You should use these for any messages that are useful in the log as well. 

     | Type   | Functions           | Notes                                                           |
     |--------|---------------------|-----------------------------------------------------------------|
     | Errors | LogError, LogErrorf | For significant errors, always reported to both log and console.|
     | Info   | Log, Logf           | Informational messages, displayed to the console and log unless --quiet. You almost never need these since you don't really need to send informational messages to the log, you'd want util.LogConsole[f] below instead.|
     | Debug  | LogDebug, LogDebugf | Debug messages that are displayed to the console if --verbose and to the log if git-lob.logverbose|

   
   * **For console output only:**

     You should use these for messages that should only go to the console because they're only useful for an interactive user. 

     | Type     | Functions                         | Notes                                                           |
     |----------|-----------------------------------|-----------------------------------------------------------------|
     | Errors   | LogConsoleError, LogConsoleErrorf | For significant errors with console input arguments etc         |
     | Info     | LogConsole, LogConsolef           | Informational messages just for the user                        |
     | Debug    | LogConsoleDebug, LogConsoleDebugf | Debug information just for the user, not the log                |
     | Progress | LogConsoleOverwrite               | Reporting progress of an operation like a download or calculation|

   * **stdout/stderr**

     You should never output to stdout/stderr directly, use the methods above. Note that
     all error messages go to stderr, and all other messages go to stdout UNLESS
     you have called LogAllConsoleOutputToStdErr(), in which case all subsequent
     output goes to stderr. Commands that are piping output to stdout (such as
     the filters) always output user messages to stderr to avoid corrupting the
     piped output. The user can also effectively suppress stdout using --quiet,
     since only errors are output and those always go to stderr.

    * **When to output errors**

     For consistency and to minimise 'noise', output error messages (LogError[f]) 
     only at the top level function which does not return an error. If the function 
     returns an error, simply add context to the error you've experienced and return 
     a new error, e.g.

     ```
     if err != nil {
       return fmt.Errorf("Context message %v: %v", ctx, err.Error())
     }
     ```

     This allows callers to decide whether the error is something that needs to be
     logged or not, based on the context. If a function doesn't return an error, 
     and failure is notable, then use LogError[f] to report it (or sometimes 
     LogConsoleError if you only want it to appear interactively)

    * **When to output debug messages**

     You can call LogDebug[f] whenever you like at any level of function, if
     the information would be useful in --verbose mode.

    * **When to output informational messages**

     When you need to provide feedback to the user at major points. Keep this
     modest, each command shouldn't really need more than 5 info messages IN
     TOTAL for a successful call, and usually less. Save the more detailed 
     output for --verbose mode (LogDebug etc).

     You'll almost always want to use util.LogConsole[f] and not Log[f] for
     informational messages, since they're not usually of any use in the log.

   



