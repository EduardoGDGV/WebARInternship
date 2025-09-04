<?php
/*
Plugin Name: Nakama Notifier
Description: Sends building updates from WordPress to Nakama server when posts change.
Version: 1.3
Author: EduardoGDGV
*/

if (!defined('ABSPATH')) exit; // No direct access

// Hook into post save (create + update)
add_action('save_post', 'nakama_notify_building_update', 10, 3);
// Hook into delete
add_action('before_delete_post', 'nakama_notify_building_delete');

function nakama_notify_building_update($post_id, $post, $update) {
    if (wp_is_post_revision($post_id)) {
        error_log("[Nakama Notifier] Ignored revision for post $post_id");
        return;
    }

    if (get_post_type($post_id) !== 'post') {
        error_log("[Nakama Notifier] Skipped non-post type: " . get_post_type($post_id));
        return;
    }

    // Only notify if post is in category 3
    $categories = wp_get_post_categories($post_id);
    if (!in_array(3, $categories)) {
        error_log("[Nakama Notifier] Post $post_id not in Buildings category. Skipping.");
        return;
    }

    // Convert stored image ID → URL
    $image_id = get_post_meta($post_id, 'image', true);
    $image_url = null;

    if ($image_id) {
        $raw_url = wp_get_attachment_url(intval($image_id));
        if ($raw_url) {
            $parsed = parse_url($raw_url);
            $image_url = $parsed['scheme'] . '://' . $parsed['host']
                . (isset($parsed['port']) ? ':' . $parsed['port'] : '')
                . dirname($parsed['path']) . '/'
                . rawurlencode(basename($parsed['path']));
        }
    }

    // Gather building data
    $building = [
        "id"     => $post_id,
        "title"  => get_the_title($post_id),
        "lat"    => (string) get_post_meta($post_id, 'lat', true),
        "lon"    => (string) get_post_meta($post_id, 'lon', true),
        "image"  => $image_url,
        "status" => get_post_status($post_id),
    ];

    error_log("[Nakama Notifier] Preparing to send building update: " . json_encode($building, JSON_UNESCAPED_SLASHES));

    // Nakama RPC endpoint with http_key
    $url = "http://nakama:7350/v2/rpc/wp_push_building?http_key=defaulthttpkey";

    // Send POST request
    $response = wp_remote_post($url, [
        'headers' => [
            'Content-Type' => 'application/json',
            'Accept'       => 'application/json',
        ],
        'body'    => json_encode(json_encode($building, JSON_UNESCAPED_SLASHES)),
        'method'  => 'POST',
        'timeout' => 10,
        'data_format' => 'body',
    ]);

    // Handle response
    if (is_wp_error($response)) {
        error_log("[Nakama Notifier] ERROR sending building update for post $post_id: " . $response->get_error_message());
    } else {
        $code = wp_remote_retrieve_response_code($response);
        $body = wp_remote_retrieve_body($response);
        error_log("[Nakama Notifier] Response from Nakama for post $post_id: HTTP $code - $body");
    }
}

function nakama_notify_building_delete($post_id) {
    if (get_post_type($post_id) !== 'post') return;

    $payload = [
        "id"      => $post_id,
        "status"  => "delete",
    ];

    error_log("[Nakama Notifier] Preparing delete notification: " . json_encode($payload, JSON_UNESCAPED_SLASHES));

    $url = "http://nakama:7350/v2/rpc/wp_push_building?http_key=defaulthttpkey";

    $response = wp_remote_post($url, [
        'headers' => [
            'Content-Type' => 'application/json',
            'Accept'       => 'application/json',
        ],
        'body'    => json_encode(json_encode($payload, JSON_UNESCAPED_SLASHES)),
        'method'  => 'POST',
        'timeout' => 10,
        'data_format' => 'body',
    ]);

    if (is_wp_error($response)) {
        error_log("[Nakama Notifier] ERROR sending delete for post $post_id: " . $response->get_error_message());
    } else {
        $code = wp_remote_retrieve_response_code($response);
        $body = wp_remote_retrieve_body($response);
        error_log("[Nakama Notifier] Delete response from Nakama for post $post_id: HTTP $code - $body");
    }
}
