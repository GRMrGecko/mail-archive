# Mail Archive

Mail Archive is a tool designed to store email copied to it for a limited period of time. It comes with a syslog server designed for use with an email spam gateway, such as Proxmox Mail Gateway, to store logs associated with an email alongside the email itself. The syslog server is designed for use with Postfix, however it can easily be adjusted to work with other email server log messages.

## Use with a spam gateway

1. Configure a (sub)domain with proper mx records for Mail Archive.
2. Setup Mail Archive with a configuration that meets your need; review the `config.go` file for available configurations.
3. Setup your email server to BCC all email to the Mail Archive server.
4. Setup your syslog server to send copies of messages over to Mail Archive.

### Example configuration for rsyslog
```
# Mail Archive syslog server
*.* @192.168.2.12:514
```

## Use as a debug mail server

Mail Archive can be used as a debug mail server for testing software fairly easily.

### Example config

```json
{
  "http_port": 1080,
  "smtp_port": 1025,
  "syslog_udp": false,
  "ui_disable_spam_reporting": true,
  "ui_disable_logs": true
}
```

After saving the config, simply start Mail Archive and configure your software accordingly.

### Use with Ruby on Rails

Edit `environments/development.rb`:

```ruby
config.action_mailer.delivery_method = :smtp
config.action_mailer.smtp_settings = { :address => '127.0.0.1', :port => 1025 }
config.action_mailer.raise_delivery_errors = false
```

### Use with PHP

You will have to use PHPMailer for this, as PHP is deisnged to use sendmail to send email. You can configure postfix to copy email to Mail Archive, but that gets complicated.

```php
$mail = new PHPMailer();

$mail->IsSMTP();
$mail->CharSet = 'UTF-8';

$mail->Host       = "127.0.0.1";
$mail->Port       = 1025;
$mail->SMTPDebug  = true;

$mail->isHTML(true);
$mail->Subject = 'Here is the subject';
$mail->Body    = 'This is the HTML message body <b>in bold!</b>';
$mail->AltBody = 'This is the body in plain text for non-HTML mail clients';

$mail->send();
```

### Use with Django

Add the following configuration to your project's `settings.py`

```python
if DEBUG:
    EMAIL_HOST = '127.0.0.1'
    EMAIL_HOST_USER = ''
    EMAIL_HOST_PASSWORD = ''
    EMAIL_PORT = 1025
    EMAIL_USE_TLS = False
```

## API

The API is available at path `/api` and is fairly feature rich.

### /ping
Test to see that the server responds correctly.

### /config

Retrieve the configuration for the web UI and current message count.

### /message_log

Pull a list of messages from the message log along with metadata.

Supported parameter:
| Parameter | Description  |
| :-------: | :----------: |
| q         | Search query |
| p         | Page number  |

### /message/{id}.log

Returns the log associated with a message.

### /message/{id}.eml

Returns the original email source.

### /message/{id}.txt

Returns the email's text body.

### /message/{id}.html

Returns the email's html body.

### /message/{id}/learn_ham

Report a message as ham to your spam reporting API.

### /message/{id}/learn_spam

Report a message as spam to your spam reporting API.

### /message/{id}

Pull metadata on a specific message.

## Building

There are a few items that must be gathered first before Mail Archive will work.

### Bower Components

[Bower](https://bower.io/) is a browser package manager which is installable via [npm](https://www.npmjs.com/). Once bower is available on your computer, simply go into the `www` directory and run the following.

```
bower install
```

### Building Mail Archive

Go into the main directory for Mail Archive and run the following to build. You will need [golang](https://golang.org/) to build.

```
go build
```
