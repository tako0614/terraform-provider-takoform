日本語: [English](../getting-started.md)

# HCL から公開済み Form を管理する

このガイドでは、AWS と Takoform のリソースを 1 つずつ含む Terraform / OpenTofu
module を紹介します。使用する設定は
[`examples/getting-started/main.tf`](../../examples/getting-started/main.tf) です。

この Provider は、公開済みの Takoform Form を HCL から管理するためのものです。
AWS、Cloudflare などの Provider と同じ立場にあり、各 source address を
`required_providers` に宣言すると、Terraform / OpenTofu が依存関係を管理します。
この Provider が Host を作成したり、実行先を選んだり、すべての Form の一覧を
提供したりするわけではありません。

## 始める前に

plan を作る前に、次を用意してください。

- Terraform または OpenTofu と、互換性のある Takoform Host。
- Host 運用者から受け取った接続先、正確な Space ID、および Host が必要とする
  場合は bearer token。Host は resource が使う正確な FormRef に対応している必要が
  あります。
- AWS Provider の通常の認証設定で利用できる AWS 認証情報、region、および選んだ
  AWS account で利用できる S3 bucket 名。

Takoform の接続先と Space は選択した Host が指定します。既定の Host や Space
はありません。bearer token を HCL や変数ファイルに書かないでください。必要なら
普段の方法で秘密情報を `TAKOFORM_TOKEN` として実行環境へ渡します。Provider は
`TAKOFORM_ENDPOINT` と `TAKOFORM_SPACE` も環境変数から読み取れます。

## 2 つの Provider を宣言する

同梱の[サンプル module](../../examples/getting-started/main.tf)を使います。
`registry.terraform.io/tako0614/takoform` と、業界標準の
`registry.terraform.io/hashicorp/aws` を宣言しています。依存グラフには S3 bucket と
`takoform_edge_kv_namespace` resource があり、通常の `depends_on` で接続しています。
片方の Provider がもう片方の resource を包んだり、credentials を転送したりは
しません。

サンプルのディレクトリで HCL の整形、初期化、検証を行います。入力と認証情報を
用意したら、apply の前に plan を確認してください。

```console
cd examples/getting-started
tofu fmt -check
tofu init
tofu validate
tofu plan
```

Terraform を使う場合は `tofu` の代わりに `terraform` を実行します。Host の接続先と
Space ID、AWS region、S3 bucket 名の 4 つは、普段使っている方法で module に渡して
ください。S3 で利用できる bucket 名を選びます。apply すると AWS account と選択した
Host の両方にリソースが作られるため、対象と変更内容を確認してから実行してください。

この resource は公開済みの
[EdgeKVNamespace Form、definition 0.1.0](https://edge.forms.takoform.com/forms/EdgeKVNamespace/0.1.0/)
に対応します。apply 前に、互換 Host がその正確な FormRef に対応していることを確認
してください。provider-neutral な Host contract は別の
[Host API ドキュメント](https://takoform.com/host-api/)にあります。そこに書かれた
contract は、Host endpoint や credentials を提供するものではありません。

## Provider の版と Form identity

サンプルの `version = "~> 4.0"` は Provider package の 4.x 系を選びます。
`tofu init` は選択した Provider package と checksum を `.terraform.lock.hcl` に記録
します。実際の module ではこの lock file を commit してください。この制約は**Form
の版を固定しません**。各 Provider release が resource type と正確な FormRef を対応
付け、resource state はその API version、kind、definition version、schema digest を
記録します。このサンプルの対応先は
`edge.forms.takoform.com/EdgeKVNamespace` の definition `0.1.0` です。完全な identity
と state の挙動は[resource reference](../resources/edge_kv_namespace.md#exact-formref)
を参照してください。

## WorkerVersion の機密入力

`TAKOFORM_TOKEN` は Host API 用の credentials です。Worker code が実行時に必要とする
アプリケーションの秘密情報とは別です。
[`takoform_worker_version`](../resources/worker_version.md) では
`required_sensitive_vars` に必要な値の名前だけを宣言します。値は実行 runner が対応
する ephemeral root-variable 経路から、Apply 時だけ Provider の `runtime_inputs` map
へ渡します。値は保存済み plan や Terraform / OpenTofu state に入りません。HCL の
literal、`vars_json`、通常の `.tfvars`、command argument、または環境変数へ値を書かない
でください。この経路を設定する前に
[Apply 単位の機密入力](../resources/worker_version.md#run-scoped-sensitive-inputs)を
読んでください。nonce、Provider instance、Apply 時の map を一致させる必要があり、
ephemeral 配送に対応しない runner ではこれらの入力を適用できません。

## 次に読むもの

- [全 resource の reference](../index.md#resource-reference) と
  [Provider から Form への対応一覧](../../forms/README.md)
- provider-neutral な契約が必要な場合は、別文書の[Takoform Host API](https://takoform.com/host-api/)
- Provider 3 から更新する前に[Provider 3 から 4 への移行ガイド](../../release/migrations/v3-to-v4.md)
