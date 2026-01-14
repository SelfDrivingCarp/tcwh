# Twich Chat Web Hook

`tcwh` is a Go service that subscribes to Twitch chat messages and posts them to
other services via WebHook calls. Currently only Discord WebHooks are supported.

## Building

The easiest way to build and run the software is as a Linux container. The
included `Dockerfile` is a two-stage build that uses a `golang` container image
to build the software and then copies artifacts into a _tiny_ `scratch` container.
Simply clone the repo and `docker build -t tcwh:latest .`.

## Running

Running the service has a few steps:

1. Preconfiguration
1. Create a Twitch Application
1. Configure the service
1. Ongoing administration

### Preconfiguration

You need a DNS name to point to you service. It can be shared with an existing
website as long as you can map tcwh to a path in the URL space, e.g.
`https://mysite.bork/tcwh/`.

You'll also need to present the web services via TLS with a valid certificate.
A Let's Encrypt certificate is fine.

### Creating a Twitch Application

First go to your [Twitch Developer Console](https://dev.twitch.tv/console).

Click _Register Your Application_.

Give your application a name. The word _Twitch_ can't appear in the name. This
name will be presented to your users when they authorize the app to join their
chat.

Set the _OAuth Redirect URLs_ to match your URL. It should be your URL for tcwh
with `oauth/cb` appended. For example: `https://mysite.bork/tcwh/oauth/cb`.
This can be changed later.

For _Category_ select `Chat Bot`.

For _Client Type_ select `Confidential`.

Click _Create_.

Now to Manage Your Application.

Note your _Client ID_, you'll need this later. Under _Client Secret_ click
`New Secret`. Note the secret and ensure this secret never gets shared or
published, e.g. don't check this secret into `git`.

### Configuring the Service

Create a directory for tcwh configuration and data. The rest of this document
assumes `/var/containers/tcwh`.

#### Service Configuration

The tcwh config is in _YAML_ and an example config can be found in [cmd/tcwh/dev-cfg.yaml](./cmd/tcwh/dev-cfg.yaml). This file has a main section and `db`, `auth`, and `twitch` sections. This configuration should be saved as `/var/containers/tcwh.yaml`.

In the main section:

 - `listen` specifies the address and port to listen on _within the container_. It's recommended that you leave this as `:8000`
 - `log-debug` controls debug logging. If set to `true` you'll get detailed log output
 - `html-path` specifies where to find the HTML content _within the container_. The defailt HTML content is baked into the container image

In the `db` section:

  - `path` specifies the path to the tcwh SQLite3 database _within the container_. The recommended value is `/data/tcwh.db` and the `/data` dir will be mounted into the container

In the `auth` section:

  - `secret` is a string from which the admin secret is derived from. This value should never be shared or published (e.g. commited to `git`). This secret is how users are given access to your tcwh service. You'll get a good secret with `head -c 16 /dev/random | base64`
  - `issuer` is the URL of your tcwh service, e.g. `https://mysite.bork/tcwh/`
  - `audience` should match `issuer`

In the `twitch` section:

  - `client-id` is the Client ID you recorded from your Twitch application
  - `client-secret` is the Client Secret from your Twitch application
  - `redirect-uri` should match the value you set in your Twitch application in _OAuth Redirect URLs_.
  - `webhook-cb-uri` should be your tcwh URL with `webhook/cb/` appended.

#### Container Configuration

It's highly recommended that you use `docker compose` to configure and run the service.

An example `docker-compose.yaml`:

```yaml
services:
    // ...
    tcwh:
        container_name: tcwh
        image: tcwh:latest
        user: "1000:1000"
        entrypoint: ["/tcwh", "/data/tcwh.yaml"]
        volumes:
            - type: bind
              source: /var/containers/tcwh
              target: /data
              read_only: false
```

You need some way to terminal TLS with a valid certification and forward traffic
to the `tcwh` container on port `8000`.

#### Generating a Token

Users access your tcwh service with a _token_. To generate a token, `POST` to
the service with the person's Twitch name and your configured auth secret. If
your auth secret was `bork bork bork` and the user's Twitch ID were `selfdrivingcarp`:

```sh
curl -X POST -H 'Authorization: bork bork bork' -d 'subject=selfdrivingcarp' https://mysite.bork/tcwh/auth/issue
```

The response will be a large blob of text. Provide this to your user through some
private channel. When the user visits your tcwh it will prompt them for their token.
Once they provide the token it will be as a cookie in their browser. They should
save the token in a password manager.

#### Bot Permissions

Before your tcwh service and support any users it needs to be authorized as a bot.
Generate a token for the Twitch user you created the Twitch application as. Log
in to your tcwh using this token and do `Connect Twitch`. This tells Twitch that
you as the owner of the bot grant it permission to connect to other user's Twitch
chats.

### Ongoing Administration

The service should just do its thing from here on. You'll need to add new users
by generating a token for them. You may need to check the logs that docker
captures to troubleshoot any issues a user is having. You might need to turn on
debug logging and restart the service.

If a user loses their token you can generate a new one. This does _not_ invalidate
previous tokens.

You can backup the service by backing up your config and the `twch.db` database:

```sh
sqlite3 tcwh.db ".backup /path/to/backup.db"
```

You can customize the HTML to make it look decent, mount your HTML dir into the
container and update the config accordingly. Users will so seldom look at the
tcwh interface that it's probably not worth the hassle.
