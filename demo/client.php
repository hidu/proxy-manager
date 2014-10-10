<?php
/**
*
 * @author duwei04@baidu.com
 * @date 2014年10月10日 下午8:18:57
* @file  client.php
* @example
* 
* php client.php -u http://news.baidu.com -p http://127.0.0.1
* 
 */
$url="http://www.baidu.com/";
$proxy="http://abc:abc@127.0.0.1:8090";

$opts=getopt("u:p:");

if(!empty($opts["u"])){
	$url=$opts["u"];
}
if(!empty($opts["p"])){
	$proxy=$opts["p"];
}

$proxyInfo=parse_url($proxy);


$ch = curl_init(); 
curl_setopt($ch, CURLOPT_URL, $url); 
curl_setopt($ch, CURLOPT_HEADER, true); 
curl_setopt($ch, CURLOPT_NOBODY, true);
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);

curl_setopt($ch, CURLOPT_PROXY,$proxyInfo["host"]);

if(!empty($proxyInfo["port"])){
    curl_setopt($ch, CURLOPT_PROXYPORT,$proxyInfo["port"]);
}
if(!empty($proxyInfo["user"]))
curl_setopt($ch, CURLOPT_PROXYUSERPWD,$proxyInfo["user"].":".$proxyInfo["pass"]);

$head = curl_exec($ch); 
print_r($head);