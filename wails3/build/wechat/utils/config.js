// 后端地址：真机预览时手机要能访问它，填本机局域网 IP（不是 127.0.0.1）
// 查看：Linux `ip addr`，Windows `ipconfig`；手机与开发机需同一网段
// 开发阶段用 http + 内网 IP（真机预览时微信"打开调试"可绕过域名校验）
module.exports = {
  BASE: 'http://192.168.1.100:28976'
};
