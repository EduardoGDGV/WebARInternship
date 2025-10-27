<?php
if (!defined('ABSPATH')) exit;

add_action('save_post', 'nakama_notify_save', 10, 3);
add_action('before_delete_post', 'nakama_notify_delete', 10);

function nakama_notify_save($post_id, $post, $update) {
    if (wp_is_post_revision($post_id)) return;
    if (get_post_status($post_id) === 'auto-draft') return;
    if (defined('DOING_AUTOSAVE') && DOING_AUTOSAVE) return;

    $type = get_post_type($post_id);
    $supported = ['event', 'asset2d', 'card', 'item', 'quiz'];
    if (!in_array($type, $supported)) return;

    $payload = [
        "id" => $post_id,
        "type" => $type,
        "title" => get_the_title($post_id),
        "status" => get_post_status($post_id),
    ];

    switch ($type) {

        case 'event':
            // lat/lon as floats
            $lat_meta = get_post_meta($post_id, 'lat', true);
            $lon_meta = get_post_meta($post_id, 'lon', true);
            $payload['coordinates'] = [
                "lat" => is_numeric($lat_meta) ? floatval($lat_meta) : 0.0,
                "lon" => is_numeric($lon_meta) ? floatval($lon_meta) : 0.0,
            ];
            // marker image (attachment -> url)
            $payload['image'] = nakama_get_image_url(get_post_meta($post_id, 'image', true));

            // unified relations (always arrays)
            $payload['requirements'] = get_post_meta($post_id, 'requirements', true) ?: [];
            $payload['rewards'] = get_post_meta($post_id, 'rewards', true) ?: [];
            break;

        case 'asset2d':
            $payload['image'] = nakama_get_image_url(get_post_meta($post_id, 'image', true));
            break;

        case 'card':
            $payload['images'] = [
                "front" => nakama_get_image_url(get_post_meta($post_id, 'front_image', true)),
                "back"  => nakama_get_image_url(get_post_meta($post_id, 'back_image', true)),
            ];
            $payload['group_card'] = (bool)get_post_meta($post_id, 'group_card', true);
            break;

        case 'item':
            $payload['images'] = [
                "2d" => nakama_get_image_url(get_post_meta($post_id, 'image_2d', true)),
                "3d" => get_post_meta($post_id, 'image_3d', true),
            ];
            break;

        case 'quiz':
            $payload['content'] = [
                "question"     => get_post_meta($post_id, 'question', true),
                "alternatives" => get_post_meta($post_id, 'alternatives', true),
                "answer"       => get_post_meta($post_id, 'answer', true),
            ];
            break;
    }

    nakama_send_payload($payload);
}

function nakama_notify_delete($post_id) {
    $type = get_post_type($post_id);
    $supported = ['event', 'asset2d', 'card', 'item', 'quiz'];
    if (!in_array($type, $supported)) return;

    nakama_send_payload([
        "id" => $post_id,
        "type" => $type,
        "status" => "delete"
    ]);
}

function nakama_send_payload($payload) {
    $response = wp_remote_post( NAKAMA_RPC_URL, [
        'headers' => [
            'Content-Type' => 'application/json',
            'Accept' => 'application/json'
        ],
        'body' => json_encode($payload, JSON_UNESCAPED_SLASHES),
        'method' => 'POST',
        'timeout' => 10,
        'data_format' => 'body',
    ] );

    if ( is_wp_error( $response ) ) {
        error_log("[Nakama Sync] Error: " . $response->get_error_message());
    } else {
        error_log("[Nakama Sync] Sent: " . json_encode($payload));
        error_log("[Nakama Sync] Response: " . wp_remote_retrieve_response_code($response) . " - " . wp_remote_retrieve_body($response));
    }
}
