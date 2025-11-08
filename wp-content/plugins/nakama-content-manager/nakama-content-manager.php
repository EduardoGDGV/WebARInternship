<?php
/*
Plugin Name: Nakama Content Manager
Description: Defines structured post types (2D Assets, Cards, Items, Quizzes, Events) and syncs them to Nakama server.
Version: 1.1
Author: EduardoGDGV
*/

if (!defined('ABSPATH')) exit;

define('NAKAMA_HTTP_KEY', 'defaulthttpkey');
define('NAKAMA_RPC_URL', 'http://nakama:7350/v2/rpc/wp_push_content?http_key=' . NAKAMA_HTTP_KEY);

require_once __DIR__ . '/includes/post-types.php';
require_once __DIR__ . '/includes/nakama-notifier.php';
require_once __DIR__ . '/includes/utils.php';
require_once __DIR__ . '/includes/admin-ui.php';
