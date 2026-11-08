// 微信小程序真机预览（零 IDE）：miniprogram-ci 生成预览二维码 → 手机微信扫码
// 用法：
//   1. 先在后端起服务（局域网可达）：scripts/mp-backend.sh
//   2. 微信公众平台 → 开发管理 → 开发设置 → 生成「代码上传密钥」，下载私钥
//   3. 填 wails3/build/wechat/project.config.json 的 appid
//   4. 运行：node scripts/mp-preview.js <私钥文件路径>
// 扫码后手机微信右上角菜单"打开调试"可绕过域名校验（访问 http://内网IP）
const ci = require('miniprogram-ci');
const path = require('path');

const privateKeyPath = process.argv[2];
if (!privateKeyPath) {
  console.error('用法: node scripts/mp-preview.js <代码上传私钥路径>');
  process.exit(1);
}

const projectRoot = path.resolve(__dirname, '../wails3/build/wechat');

(async () => {
  const project = new ci.Project({
    appid: require(path.join(projectRoot, 'project.config.json')).appid,
    type: 'miniProgram',
    projectPath: projectRoot,
    privateKeyPath: path.resolve(privateKeyPath),
    ignores: ['node_modules/**/*']
  });
  const previewResult = await ci.preview({
    project,
    desc: 'HomeCast 真机预览',
    setting: { es6: true, minify: true },
    qrcodeFormat: 'terminal', // 终端直接显示二维码（或用 'image' 输出图片）
    onProgressUpdate: console.log
  });
  console.log('预览二维码已生成，用手机微信扫码打开。');
  console.log('若请求被拦：手机微信打开小程序后右上角菜单 → 打开调试 → 重新搜索。');
})().catch((e) => {
  console.error('预览失败:', e);
  process.exit(1);
});
