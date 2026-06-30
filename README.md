# cdkm

Fan AWS CDK operations across many accounts in parallel — from your laptop.

`cdkm deploy --group prod` synthesizes and deploys to every account in the
`prod` group concurrently, each in an isolated `cdk.out/<account>`, with a live
status table and safety gates on destroy.

## Status

Early development. See `docs/superpowers/specs/` for the design.

## License

Apache-2.0
