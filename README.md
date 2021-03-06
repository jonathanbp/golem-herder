# golem-herder

[![Build Status](https://travis-ci.org/Webstrates/golem-herder.svg?branch=develop)](https://travis-ci.org/Webstrates/golem-herder) [![Go Report Card](https://goreportcard.com/badge/github.com/Webstrates/golem-herder)](https://goreportcard.com/report/github.com/Webstrates/golem-herder) [![Codacy Badge](https://api.codacy.com/project/badge/Grade/db0b35476fa84181b81d2fa7d06ae27f)](https://www.codacy.com/app/Webstrates/golem-herder?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=Webstrates/golem-herder&amp;utm_campaign=Badge_Grade)

The golem-herder is a process which manages golems, minions and daemons. A common theme here is that all of them are external processes which are slaved to a webstrate in some manner.

  * [Concepts](#concepts)
    * [Golems](#golems)
    * [Minions](#minions)
    * [Daemons](#daemons)
  * [Installation](#installation)

## Concepts

### Golems

A **golem** is process which connects to a specific webstrate. One golem pr webstrate. It's specifically a docker container running a chrome-headless showing a webstrate. A golem can interact with the webstrate as any normal (browser) client on a webstrate. It therefore forms the glue which allows other procesesses to inspect and manipulate a webstrate. To attach a golem on webstrate two steps are requied.

  1. First the golem-herder must be told to spawn a golem for the webstrate. This is done by issuing a http request to `http(s)://<location-of-herder>/golem/v1/spawn/<id-of-webstrate>`. Normally you'd not do this manually but rather have a script on the webstrate itself handle initialization. Simply include the script at `http(s)://<location-of-herder>/` in the webstrate. `<location-of-herder>` is `emet.cc.au` unless you're running your own golem herder.

  2. The golem will bootstrap itself with code found in the dom element with the query-selector `.golem,#emet`. This code will be run in the headless-chrome. You'd probably want to set up a few websocket connections to the herder to listen for connecting ad-hoc minions, to spawn new controlled minions or daemons.

### Minions

A **minion** is a (light-weight) process which augments the more heavy-weight golems with functionality. A golem can interact with two different types of minions.

#### Ad-hoc

The first is an **ad-hoc** minion which is a process that is not controlled by the herder but rather runs your own hardware. In order to connect a process on your own machine to a golem in a webstrate you should just connect a websocket in your ad-hoc minion to `http(s)://<herder-location>/minion/v1/connect/<webstrate-id>?type=<minion-type>`. The `minion-type` is provided to the golem to let you determine its behaviour towards different types of ad-hoc minions. Once the minion and the golem are connected you can implement a comm protocol to fit your objective.

#### Controlled

The second is a **controlled** minion spawned and controlled by the golem. This type of minion is expected to be shortlived (max 30 seconds) and is essentially a mechanism by which an external environment may be used to execute code. It can be a piece of python or ruby code that calculates a result which is returned and used by the golem (and webstrate). Another example is a [minion which converts latex to pdf](https://github.com/Webstrates/minion-latex) to be shown in the browser.

A controlled minion is usually spawned by the golem by letting it send an http POST request to the herder on `http(s)://<herder-location>/minion/v1/spawn`. The request should contain a number of form variables:

 * A form variable with the `env` name determines the environment in which the minion is run. This is translated to a docker image. Current options are `ruby`, `python`, `latex` and `node`. This variable must be set.
 * A form variable with the `output` name determines the file which should be returned as output from the minion. If omitted then a JSON object with `stdout` and `stderr` is returned.
 * Any other form variables are treated as files to be written to the container prior to executing it. E.g. the form variable with `main.sh` and value `echo 'hello'` will get written to the "main.sh" file with "echo 'hello'" as content. The `main.sh` file is normally the file that is executed when the container is started but this is determined by the container image (as selected by the `env` variable).

### Daemons

A **daemon** is conceptually the same as a *controlled minion*, however a daemon my be longlived. In order to spawn a daemon you must have a token. Tokens can be generated from the command line with

    ./golem-herder token -e <email address> [-c <number of credits>]

The golem-herder must not be running when doing this. Or using the POST method explained below.

 * **Start a daemon** by sending an POST request to: `http(s)://<herder-location>/daemon/v1/spawn`. The request should contain the following form variables:
   - `name` is the name of daemon - this must be unique for the token used
   - `image` is the docker image that contains the daemon code
   - `ports` are a list of ports (json-formatted list of strings) which should be opened in the container
   If the daemon is successfully spawned then a json object describing the daemon and how its ports are mapped will be returned.

 * **List daemons** by sending a GET request to `http(s)://<herder-location>/daemon/v1/ls`

 * **Kill a daemon** by sending a GET request to`http(s)://<herder-location>/daemon/v1/kill/<name-of-daemon>`

 * **Attach to a deamons stdout/err/in** via websockets `ws(s)://<herder-location>/daemon/v1/attach/<name-of-daemon>`

 * **Access exposed port of daemon through reverse proxy** `ws(s)://<herder-location>/daemon/v1/attach/<name-of-daemon>` (The reverse proxy will be to the first defined port in `ports`. E.g. if `ports` is defined as [80, 8080], the URL will proxy the user to port 80 in the container.)

* **Generate token** by sending  POST request to `http(s)://<herder-location>/token/v1/generate`. The request should contain the following form variables:
   - `password` The password specified using `--token-password` to the golem-herder on the command line when starting it.
   - `email` The email address (or any identifier) for the owner of the token.
   - (optional) `credits` The amount of credits on the token. Defaults to 30000.

* **Inspect token** by sending a GET request to `http(s)://<herder-location>/token/v1/inspect/<token>`. This will give you a JSON object back with the associated email, the remaining credits, (and the token).

For all commands (except token generation) you need to supply a token ([JWT](https://jwt.io)). This can be done in the header in the format `Authorization: bearer <token>` or as a query param e.g. `...?token=<token>`.

Credits are associated with email addresses not tokens themselves, so when generating a token, the credits specified will added to specified email address' credit score and be usable by all existing and newer tokens.

## Installation

If you want a local golem-herder installed you can do this by downloading a version matching your os/architecture at the [releases](https://github.com/Webstrates/golem-herder/releases) page. See the internal doc by e.g. running:

```sh
> golem-herder_linux_amd64 -h
```

You can create a self-signed key-pair for testing with:

```sh
> openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -nodes -days 365 -subj /CN=localhost
```
