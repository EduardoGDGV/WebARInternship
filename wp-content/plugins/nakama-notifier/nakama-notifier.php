<?php
/*
Plugin Name: Nakama Notifier
Description: Sends building updates from WordPress to Nakama server when posts change.
Version: 1.0
Author: YourName
*/

if (!defined('ABSPATH')) exit; // No direct access

// Hook into post save (create + update)
add_action('save_post', 'nakama_notify_building_update');
// Hook into delete
add_action('before_delete_post', 'nakama_notify_building_update');

function nakama_notify_building_update($post_id) {
    // Only run for normal posts (you can filter by custom type/category)
    if (get_post_type($post_id) !== 'post') return;

    // Example: only notify if in category ID 3 (Buildings)
    $categories = wp_get_post_categories($post_id);
    if (!in_array(3, $categories)) return;

    // Gather building data
    $building = [
        "id"    => $post_id,
        "title" => get_the_title($post_id),
        "lat"   => get_post_meta($post_id, 'lat', true),
        "lng"   => get_post_meta($post_id, 'lng', true),
        "image" => get_the_post_thumbnail_url($post_id, 'full'),
        "status" => get_post_status($post_id),
    ];

    // Nakama RPC endpoint (Docker internal hostname, not localhost!)
    $url = "http://nakama:7350/v2/rpc/wp_push_building";

    // Send POST request
    $response = wp_remote_post($url, [
        'body'    => json_encode($building),
        'headers' => [
            'Content-Type' => 'application/json',
            'Authorization' => 'Basic ' . base64_encode('defaultkey:'),
        ],
        'timeout' => 5,
    ]);

    if (is_wp_error($response)) {
        error_log("Nakama Notifier error: " . $response->get_error_message());
    }
}
