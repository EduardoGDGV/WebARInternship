<?php
if (!defined('ABSPATH')) exit;

// Convert attachment ID to full URL
function nakama_get_image_url($id) {
    if (!$id) return null;
    $raw_url = wp_get_attachment_url(intval($id));
    if (!$raw_url) return null;
    $parsed = parse_url($raw_url);
    return $parsed['scheme'] . '://' . $parsed['host']
        . (isset($parsed['port']) ? ':' . $parsed['port'] : '')
        . dirname($parsed['path']) . '/'
        . rawurlencode(basename($parsed['path']));
}
