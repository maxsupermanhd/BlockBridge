# Configuration tree

Config is loaded from `config.json` (or from envvar `BLOCKBRIDGE_CONFIG`)

| Path | Type | Can be changed | Default | Description |
| --- | --- | --- | --- | --- |
| `DatabaseFile` | string | No | Error? | Path to sqlite3 database, will be created if does not exist (only folders must exist) |
| `ChannelID` | string | Yes | `` | From what channels accept chat messages (d->m), comma separated |
| `AddPrefix` | bool | Yes | `false` | Add formatting to accepted chat messages (d->m) |
| `Discord`.`Token` | string | No | Error | Discord token |
| `Discord`.`AppID` | string | No | Error? | Discord application ID to initialize slash commands in |
| `Discord`.`GuildID` | string | No | Error? | Discord guild ID to initialize slash commands in |
| `NameOverridesPath` | string | No | Disabled | Load name overrides JSON from this path (see below) |
| `FontPath` | string | No | `Minecraft-Regular.otf` | TTF/OTF font path to render tab with |
| `LogsFilename` | string | No | `logs/chatlog.log` | Path to log file |
| `LogsMaxSize` | int | No | `10` | Maximum size in megabytes of the log file before it gets rotated |
| `AddTimestamps` | bool | Yes | `false` | Add `[02 Jan 06 15:04:05]` ` formatted timestamp to messages (m->d) |
| `CredentialsRoot` | string | Yes | `cmd/auth/` | Path to folder from where to take account information |
| `AllowedChat` | string | Yes | Not set | List of comma separated user IDs that are allowed to chat (d->m), if not set whitelist is disabled |
| `AllowedSlash` | string | Yes | Not set | List of comma separated user IDs that are allowed to execute slash commands (d->m) (actual minecraft ones, not discord ones, essentially allows "messages" starting with `/` to be sent to the server), if not set whitelist is disabled |
| `MCUsername` | string | On relog | `FlexCoral` | Username to log in with |
| `ServerAddress` | string | On relog | `localhost` | Address to connect to (IP or domain) (port after `:`) |
