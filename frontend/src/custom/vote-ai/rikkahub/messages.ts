export default {
  zh: {
    downloadApp: '下载官方 App（2.5.5）', intro: '安装 RikkaHub 后，导入此密钥的配置，即可通过 Responses 接口聊天。',
    loading: '正在读取此密钥可用的模型…', loadError: '无法读取模型。请检查密钥状态、分组或网络后重试。', retry: '重试', empty: '此密钥暂时没有可导入的模型。',
    model: '选择导入的模型', vision: '支持文本和图片输入', textOnly: '文本输入', effort: '预设思考强度：{value}',
    private: '二维码包含你的 API Key，请勿转发或公开。二维码仅在浏览器中生成。',
    generate: '显示个人二维码', generating: '生成中…', qrError: '生成失败，请重试或选择其他模型。', qrAlt: '个人 RikkaHub 配置二维码，请勿分享', save: '保存二维码', hide: '隐藏二维码',
    step1: '手机打开 RikkaHub：设置 → 提供商 → 导入 → 扫描二维码。', step2: '只有一台手机时，保存二维码，再从 RikkaHub 导入界面选择相册图片。',
    step3: '回到聊天页面选择新导入的 Responses 提供商和模型，发送文字或图片测试。',
    snapshot: '导入会新增配置，不会自动替换旧提供商。模型权限和额度以服务端为准；预设思考参数可能覆盖 App 的思考档位。'
  },
  en: {
    downloadApp: 'Download official app (2.5.5)', intro: 'Install RikkaHub and import this key’s configuration to chat using the Responses API.',
    loading: 'Loading models available to this key…', loadError: 'Could not load models. Check your key, group or connection and retry.', retry: 'Retry', empty: 'No importable models are available to this key.',
    model: 'Model to import', vision: 'Text and image input', textOnly: 'Text input', effort: 'Preset reasoning effort: {value}',
    private: 'This QR code contains your API key. Keep it private. It is generated only in your browser.',
    generate: 'Show personal QR code', generating: 'Generating…', qrError: 'Generation failed. Retry or choose another model.', qrAlt: 'Private RikkaHub configuration QR code', save: 'Save QR code', hide: 'Hide QR code',
    step1: 'In RikkaHub, open Settings → Providers → Import → Scan QR code.', step2: 'On a single phone, save this image and choose it from the gallery in the import dialog.',
    step3: 'Select the newly imported Responses provider and model in chat, then try a message or image.',
    snapshot: 'Import adds a configuration; it does not replace old providers. Server permissions and quotas still apply. Preset reasoning parameters may override the app’s effort selector.'
  }
}
