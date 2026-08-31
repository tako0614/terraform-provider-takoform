---
title: 所有範囲
description: Takoform の各境界を所有する repository と runtime。
---

# 所有範囲

境界ごとに一つの authority を置くことで、Takoform は移植性を保ちます。
Provider は contract を projection できますが、利用する contract 自体の
authority にはなりません。

| 境界                                                  | 所有者                                                                                   | 決めること                                                                                                                             |
| ----------------------------------------------------- | ---------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Normative Specification、schema、conformance、receipt | [`takoform`](https://github.com/tako0614/takoform)                                       | Host API の意味、identity grammar、canonicalization、package / trust format、履歴資料。同じ repository が Core `v1.1.0` を公開します。 |
| Form family と exact FormRef                          | [`takoform-forms`](https://github.com/tako0614/takoform-forms)                           | family group、`formId`、`definitionVersion`、Form / package bytes、fixture、signature、deprecation、revocation。                       |
| Terraform / OpenTofu Provider                         | [`terraform-provider-takoform`](https://github.com/tako0614/terraform-provider-takoform) | typed resource、state、import の mapping と不変の Provider release 履歴。                                                              |
| host runtime                                          | host の operator と実装                                                                  | capability support、配置、scaling、資格情報、routing、recovery、billing、quota、SLA、live catalog。                                    |
| deployment と secret                                  | operator                                                                                 | endpoint、space、資格情報、capacity policy、production state。Form や Provider の公開 metadata にはしません。                          |

## この分担から分かること

- Provider release は Form を公開せず、Host API lane も変えません。
- Core release は Provider version や Form を選びません。
- Form の意味論を変更するときは、Form publisher が `definitionVersion` を変更します。
- host は Host Support Profile で、対応する exact FormRef を知らせます。
- このサイトは事実を要約しますが、live capability、資格情報、deployment state は所有しません。

現行の wire identity は literal な `forms.takoform.com/v1` です。サイトが
`/v1.1` を作ったり、番号付き Specification receipt を他の所有者の release
authority として扱ったりすることはありません。

## source と履歴

[バージョンモデル](/ja/docs/versions.html)に現行 4 stream を、[仕様資料（履歴）](/ja/spec/)
と[リリース資料](/ja/docs/history.html)に保持された source をまとめています。
旧 URL には履歴であることを示す案内があります。

<StatusNote />
