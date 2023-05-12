DROP TABLE IF EXISTS `tunnel`;
CREATE TABLE `tunnel` (
                          `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '编号',
                          `name` varchar(60)  NOT NULL DEFAULT '' COMMENT '描述',
                          `input` varchar(60)  NOT NULL DEFAULT '0.0.0.0:6666/tcp' COMMENT '输入监听地址',
                          `output` varchar(60) NOT NULL DEFAULT '0.0.0.0:6666/tcp' COMMENT '输出地址',
                          `in_decrypt_mode` varchar(60) NOT NULL DEFAULT 'gcm' COMMENT '输入解密方式',
                          `in_decrypt_key` varchar(60) NOT NULL DEFAULT 'gotun' COMMENT '输入解密密钥',
                          `out_crypt_mode` varchar(60) NOT NULL DEFAULT 'gcm' COMMENT '输出加密方式',
                          `out_crypt_key` varchar(60) NOT NULL DEFAULT 'gotun' COMMENT '输出加密密钥',
                          `out_mux_conn` int(11) NOT NULL DEFAULT '20' COMMENT '输出复用连接数，输出地址非本程序时，要为0',
                          `in_extra` text NOT NULL DEFAULT '' COMMENT '输入额外参数',
                          `out_extra` text NOT NULL DEFAULT '' COMMENT '输出额外参数',
                          PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='隧道表';