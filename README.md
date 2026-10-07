# Home Clipboard Canvas

家庭内LANで使う、一時的な共有クリップボードキャンバスです。テキストや画像をブラウザから置き、別の端末へコピーできます。カード、画像、表示位置はサーバーのRAMだけに保持し、初期設定では作成から1時間で削除します。サーバーを終了すると内容と認証セッションが消えます。

Go製のサーバーにReactのWeb UIを埋め込み、`dist/HomeClipboardCanvas.exe` 単体で配布できます。利用端末にGoやNode.jsは不要です。

## 開発PCで試す

PowerShellでプロジェクトのルートから実行します。

```powershell
.\scripts\dev.ps1
```

ビルド後、[http://127.0.0.1:8080](http://127.0.0.1:8080) を開きます。`tools/dev-token.txt` の共有トークンを入力してください。生成したトークンファイルは正常終了時に削除します。Ctrl＋Cでサーバーを終了します。強制終了でファイルが残った場合は手動で削除できます。

開発モードはloopback IPへのbindしか許可しません。Startup、レジストリ、Firewall、証明書ストア、hosts、DNSは変更しません。開発中に `install-startup` を実行しないでください。

## 使い方

| 操作 | 方法 |
| --- | --- |
| テキストや画像を貼り付け | Ctrl＋V、または「貼り付け」 |
| スマートフォンから共有 | 「テキストを置く」の入力欄に貼り付け、または「画像を選ぶ」 |
| コピー | カードの「コピー」。選択してCtrl＋Cも使用可能 |
| 画像を取り出す | 「コピー」、または「開く」「保存」 |
| カード移動 | カード上部をドラッグ。フォーカスしたカードは矢印キーでも移動 |
| カードサイズ変更 | 選択したカードの右下をドラッグ。サイズ変更ボタンにフォーカスして矢印キー |
| 表示位置の移動 | 背景をドラッグ、スマートフォンでは2本指 |
| 拡大・縮小 | ホイール、倍率ボタン、スマートフォンでは2本指 |
| 削除 | カードの×、または選択してDelete |
| 全削除 | 「すべて削除」の確認ダイアログで実行 |
| 長押し | カードのコピーメニューを表示 |

クリップボードに画像とテキストの両方がある場合、画像を優先します。PNG・JPEG・GIFを受け付け、元画像を縮小・再圧縮せず保存します。JPEG・GIFをClipboard APIへコピーするときだけPNGに変換します。GIFのコピーは静止画になります。元のGIFは「開く」「保存」で取得できます。

ブラウザのクリップボード権限や対応状況により、自動読取・画像コピーが利用できないことがあります。その場合は入力欄への貼り付けや画像の「開く」「保存」を使えます。ブラウザからローカル端末のクリップボードへ書き込むには、実運用ではクライアントから信頼されるHTTPS証明書が必要です。

ほかの端末のカード操作はリアルタイムで同期します。表示位置は各端末で独立し、他端末の操作で画面が動くことはありません。操作終了後800ms、ページ非表示・終了時に最新の操作時刻を添えて保存し、次回アクセス時に復元します。新しいカードがあるときは「新しいカードへ」から移動できます。ブラウザには前回利用の時刻だけを保存し、カード内容は保存しません。

## ビルドとテスト

必要環境はWindows、Go 1.25以上、Node.js 22.12以上、npmです。この作業環境ではGoを `tools/go` にプロジェクト内展開しています。スクリプトはPATH上のGo、次に `tools/go/bin/go.exe` を探します。

```powershell
.\scripts\build.ps1
.\scripts\test.ps1 -SkipBuild
```

ビルドは `npm ci` → TypeScript型検証 → Viteビルド → Go埋め込み → Goテスト・vet → EXE生成の順です。追加の検証として、実時間TTLとEXE再起動を確認できます。

```powershell
$env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $PWD 'tools\browsers'
node .\scripts\acceptance.mjs
```

AcceptanceはEXEを `tools/acceptance` にコピーしてlocalhostだけで起動します。再起動後の消去を確認し、TTLをテスト用に1分へ設定して、実時間で期限切れ通知を待ちます。通常は1〜2分かかります。結果は `artifacts/acceptance.json` に出力し、プロセスとテスト設定は終了時に片付けます。標準の1時間TTLはGoの時計を差し替えたテストで検証します。

`test.ps1` はテスト用Chromiumを `tools/browsers` に配置します。スクリーンショットは `artifacts`、失敗時のトレースは `frontend/test-results`、HTMLレポートは `frontend/playwright-report` に出力します。これらはGit管理対象外です。

画面を編集してViteの開発サーバーを使う場合は、`dev.ps1` のサーバーを起動した上で別のターミナルで実行できます。

```powershell
cd frontend
npm run dev
```

Viteも127.0.0.1だけで起動し、APIとWebSocketを8080へproxyします。

## 実際のサーバーPCに配置する

この手順は開発PCでは実行しません。

1. `HomeClipboardCanvas.exe`、`config.json`、証明書、秘密鍵を同じフォルダへ置きます。
2. `config.example.json` をコピーして `config.json` を作ります。
3. `HomeClipboardCanvas.exe generate-token` で生成した32byteトークンを `config.json` の `token` に入れます。環境変数 `HCC_TOKEN` でも指定できます。
4. 利用するホスト名・IPに適合し、各利用端末から信頼される証明書を用意して、`certificate` と `privateKey` を設定します。
5. サーバーPCで明示的に起動します。

```powershell
.\HomeClipboardCanvas.exe serve --config .\config.json
```

証明書・秘密鍵・トークンが不正または未設定なら起動を拒否します。証明書ファイルの指定も可能です。

```powershell
.\HomeClipboardCanvas.exe serve --config .\config.json --cert .\certificate.pem --key .\private-key.pem
```

Firewall設定や名前解決の自動変更はありません。実運用のサーバーとLANに合わせて管理者が設定してください。`allowedNetworks` は標準のprivate/loopback CIDRに**追加**する設定です。外部公開向けの設計ではありません。

初回アクセスはトークン入力画面、または `https://サーバー:8443/?token=共有トークン` を使います。共有URLからのトークンはフロントエンドの最初の処理で除去し、POSTで認証します。認証後はSecure・HttpOnly・SameSite=Strict Cookieを利用します。共有URL自体は秘密として扱ってください。

## StartupとCLI

サーバーPCで、証明書とトークンを含む有効な**config.json**を用意した後に登録できます。登録するショートカットの引数は `serve --config "絶対パス"` です。登録時だけの環境変数や `--cert` の値はショートカットに保存しません。

```powershell
.\HomeClipboardCanvas.exe status --config .\config.json
.\HomeClipboardCanvas.exe install-startup --config .\config.json
.\HomeClipboardCanvas.exe uninstall-startup --config .\config.json
.\HomeClipboardCanvas.exe version
```

Startupショートカットは現在のWindowsユーザーのStartupフォルダに作ります。同じ登録は重複させず、同名の無関係なショートカットの上書き・削除を拒否します。Windowsサービスは作りません。`status` は設定とショートカットの状態を表示します。プロセスが実際に稼働中かを判定するコマンドではありません。

## データと制限

初期値は100カード、テキスト1MiB、画像10MiB、カード内容全体200MiB、TTL60分です。上限超過はHTTP 413です。画像は復号可能なPNG・JPEG・GIFを検証し、4,000万画素を超えるものを拒否します。SVG・HTML・任意ファイルは扱いません。

TTLは作成時から数えます。移動・サイズ変更では延長しません。30秒ごとの検出に加え、状態取得・画像取得・作成・更新時にも期限切れを除去します。削除・期限切れ・全削除時には画像バイナリも解放します。200MiBの制限は保持データとカードあたり1KiBの管理予算であり、Goランタイム・画像復号・HTTP転送を含むプロセス全体のRSS上限ではありません。RAMからの解放はGCに委ね、物理メモリのゼロ化やOSのページファイル消去は保証しません。

ログには操作の種類とサイズだけを出力します。Clipboard内容、共有トークン、Cookie、URLは記録しません。設定・証明書・秘密鍵・トークン、生成物はGit管理対象外です。DB、履歴ファイル、Service Workerは使いません。

設計とAPIは [docs/design.md](docs/design.md)、検証の範囲は [docs/verification.md](docs/verification.md) を参照してください。
