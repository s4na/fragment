# fragment

fragmentは、ローカルのドキュメントフォルダをCLI、MCP、Web UIから共通して読み書きするアプリケーションです。

プロジェクトごとに `flow` と `stock` のフォルダを持ち、自由な内容のMarkdownを保存します。チャットなどをflowに残し、AIとの対話で保存を指示した内容をstockに溜めていきます。

ghqのリポジトリやブランチを切り替えても同じ保存先を参照し、Goの共通コアで操作します。PreactのWeb UIでは、保存されたMarkdownを整形して閲覧できます。

## ビルドと検証

macOSのローカルディスクを対象に検証しています。開発にはGo 1.26.3以降、Node.js 22.12以降、pnpm、Socket Firewallの `sfw` が必要です。ビルド済みの実行ファイルは、Node.jsやpnpmなしで利用できます。

```sh
make build
make check
```

`make build` はPreactの画面をビルドし、埋め込んだ実行ファイルを `bin/fragment` に作成します。フロントエンドのpnpmコマンドは `sfw` 経由で実行します。PRを出す前には `make check` を実行してください。lintとtestをローカルでまとめて実行し、どれか1つでも失敗するとコマンドも失敗します。

`make test` はGoのrace検出付きテストと `go vet`、Web UIの自動テストを実行します。Go側は文書の読み書き、同時更新の競合、CLI・MCPの連携、HTTP、MarkdownのHTML表示などを確認します。Web側は文書取得・更新の通信、認証情報、競合時の再取得・リトライの案内、通信失敗の扱いを確認します。Webのテストは[Vitest](https://vitest.dev/guide/)で一回実行して終了し、ファイル監視は行いません。Goだけなら `make test-go`、Webだけなら `make test-web` で実行できます。

Goの書き込みテストは実行中だけの一時フォルダを使い、終了後に削除します。利用者のDocuments配下の文書は変更しません。通常の `make check` はCodexの実際の推論を実行しません。

`make lint` はローカルでコードをチェックします。Goの整形漏れ（`gofmt`）と不具合の兆候（`go vet`）、Web UIの問題（Biome）とTypeScriptの型エラーを調べ、問題があれば失敗します。コードの自動修正やアプリの起動は行いません。GitHub Actionsの設定は追加せず、PRを出す前に手元で実行する使い方です。Biomeは[公式の導入手順](https://biomejs.dev/installation/quick-start/)に従い、バージョンを固定しています。

Go側だけなら `make lint-go`、Web UI側だけなら `make lint-web` で実行できます。Goのチェックでも、画面を実行ファイルに埋め込むために先にWeb UIをビルドします。Goの整形漏れは、表示されたファイルに `gofmt -w <ファイル>` を実行して直してください。

## 使い始める

```sh
bin/fragment project create "新サービス"
printf '# 概要\n\n自由なMarkdown本文。\n' | bin/fragment doc create \
  --project "新サービス" --kind stock --path "概要.md"
bin/fragment serve
```

`serve` が表示するURLをブラウザで開くと接続できます。最初の接続で起動時の一回限りのトークンを使い、その後はローカルのセッションCookieで認証します。別のブラウザから接続する場合はサーバーを再起動して新しいURLを使ってください。

設定なしでは `~/Documents/fragment` を共通保存先に使い、最初の文書操作・MCP・Webサーバーの起動時に必要なフォルダとロックファイルを自動作成します。`config show` で保存先を確認できます。変更したい場合だけ `bin/fragment config init /absolute/path/to/documents` を実行してください。

保存先の名前は自由です。設定を同じOSユーザーで共有するため、リポジトリやブランチを切り替えても同じデータを読み書きします。設定を変更したら常駐しているMCPとWebサーバーを再起動してください。`config init` は保存先の設定を変更しますが、既存の文書を移動しません。

## CLIとMCP

CLIはJSONを標準出力へ返します。本文は標準入力、または `--file` で指定したファイルから入力できます。

```sh
bin/fragment project list
bin/fragment doc list --project "新サービス" --kind flow
bin/fragment doc get --project "新サービス" --kind stock --path "概要.md"
bin/fragment doc update --project "新サービス" --kind stock --path "概要.md" \
  --revision "取得結果のrevision" --file ./updated.md
bin/fragment mcp
```

AIクライアントには、ビルド済みの実行ファイルの絶対パスと引数 `mcp` を登録してください。stdioで `list_projects`、`create_project`、`list_documents`、`get_document`、`create_document`、`update_document` を提供します。

```json
{
  "mcpServers": {
    "fragment": {
      "command": "/absolute/path/to/fragment/bin/fragment",
      "args": ["mcp"]
    }
  }
}
```

## Web UIのCodexチャット

macOSではデスクトップアプリ同梱のCodex CLIを優先し、その他の場合はPATHの `codex` を使います。ローカルのCodex CLIをインストールし、`codex login` を完了してから `bin/fragment serve` を起動してください。実行ファイルがPATHにない場合は `serve --codex /absolute/path/to/codex` で指定できます。文書の閲覧・手動編集・CLI・MCPはCodexなしでも利用できます。

プロジェクトのチャット欄から、たとえば「この内容をstock/概要.mdに保存して」「既存の説明を保って、この決定事項を追記して」と指示します。Goが `codex app-server --listen stdio://` を起動し、応答を画面に表示します。会話をflowの `chats/<時刻>-<乱数>.md` に保存し、文書操作はCLI・MCPと同じGoの共通コアで実行します。stockへの保存・更新は利用者の指示に従います。

更新前にはCodexが本文とリビジョンを取得します。競合するとレスポンスとチャットに「最新データを再取得し、既存の内容を確認して変更案を作り直し、リトライしてください」と表示します。取得済みリビジョンは破棄し、`fragment_get_document` を呼び直すまで更新を拒否します。Codexは再取得した本文に指示を反映して再試行し、意図の衝突を解消できない場合は利用者に確認を求めます。

手動編集も、競合時に最新本文を再取得して下書きと一緒に表示します。下書きを編集して既存内容を反映し、確認した変更案をリトライしてください。古い本文に新しいリビジョンだけを差し替える自動再送は行いません。

同一プロジェクトは一回の応答、全体で最大4回の応答を並行実行できます。応答の期限は5分、入力は32KiB、会話は256KiBです。画面を再読み込みすると進行中の会話に戻れます。サーバーの再起動後は新しい会話になりますが、過去の会話はflowから閲覧できます。各ターンは過去の会話を渡す新しいephemeral threadを使用します。

Codexの既存の認証・モデル設定を使います。設定モデルがapp-serverの一覧にない場合は標準モデルを選び、その選択をチャットに表示します。`serve --model <モデル名>` で明示することもできます。文書の内容や会話は推論のためにそのモデルの提供先へ送信されます。チャット用スレッドはread-only sandboxを使い、シェル・直接編集・別のMCP・コネクター・サブエージェントを無効にし、fragmentの文書ツールでのみ更新します。dynamic toolsはexperimental APIで、プロトコルはCodex CLI 0.142.3とデスクトップ同梱の0.162.0-alpha.17.2で接続し、実際の文書更新・競合回復は後者で検証しています。[Codex App Serverの公式ドキュメント](https://learn.chatgpt.com/docs/app-server) に接続手順とプロトコルがあります。

CLIや外部MCPで競合した場合も最新の本文を取得し、変更内容を検討してから新しいリビジョンで更新してください。

## 初期版の範囲

文書の作成・取得・更新、一覧のページング、Markdownの表示・編集・プレビューを提供します。保存先の `.fragment/write.lock` にOSの排他ロックを取得し、ロック内で取得時の内容ハッシュを比較します。文書は一時ファイルから置き換え、古い版による上書きを拒否します。

- 文書の上限は8MiB、一覧は一回100件（最大1000件）、書き込みロックの待機は5秒です。
- Webサーバーはloopbackのみを使用します。生HTMLは表示せず、HTMLをサニタイズします。
- flowとstock内の相対Markdownリンクを解決します。画像配信やMermaid表示、改名、移動、削除、同期、編集履歴は未対応です。
- 安全な同時書き込みの範囲はアプリケーションの入口間です。外部エディターや同期ソフトはOSロックに従わない場合があります。

通常の設定は `os.UserConfigDir()` 配下の `fragment/config.json` にあります。検証用には絶対パスの `FRAGMENT_CONFIG_DIR` で設定ディレクトリを切り替えられます。CLI・MCP・Webサーバーには同じ値を渡してください。この変数を明示した場合は指定先の設定ファイルが必要で、未設定のまま通常の保存先へ切り替わることはありません。利用者の文書とアプリケーションのソースコードは別の場所に保管します。

- [共通ドキュメントストアのデザインドック](docs/design-doc.md)
