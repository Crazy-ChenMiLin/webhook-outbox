# Security

The relay makes HTTP requests to URLs your customers give you, signed with secrets
stored in your database. Both parts run with the database credentials you give them.

If you find a vulnerability, for example a way past the private address check, a way
to make the relay sign a body the application did not queue, or a timing leak in
`Webhook::verify()`, do not open a public issue. Report it privately through
[GitHub](https://github.com/IanFoxDev/webhook-outbox/security/advisories/new), or write
to ianfoxdeveloper@gmail.com.

Some things are by design and are not vulnerabilities:

- `RELAY_ALLOW_PRIVATE_NETWORKS=true` turns the private address check off.
- Secrets are stored in `webhook_endpoints` as they are, because the relay signs with
  them. Anyone who can read that table can sign as you.
- The relay's HTTP port serves metrics and health checks without authentication. Do
  not expose it outside your network.
- Payloads are stored and sent as they are. Do not put secrets or card data into them.

## Supported versions

Fixes go into the latest release only. Until 1.0 that is the latest `0.x` tag.
