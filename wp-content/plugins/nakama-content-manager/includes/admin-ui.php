<?php
if (!defined('ABSPATH')) exit;

add_action('enqueue_block_editor_assets', 'nakama_enqueue_event_ui');

function nakama_enqueue_event_ui() {
    $asset_path = plugin_dir_url(__FILE__) . '../js/event-ui.js';
    wp_enqueue_script(
        'nakama-event-ui',
        $asset_path,
        ['wp-api-fetch', 'wp-edit-post', 'wp-data', 'wp-element', 'wp-components', 'wp-i18n', 'wp-plugins'],
        filemtime(plugin_dir_path(__FILE__) . '../js/event-ui.js'),
        true
    );

    $config = [
        'nonce' => wp_create_nonce('wp_rest'),
        'rest_base' => rest_url('wp/v2'),
        // Which post types the UI will search
        'postTypes' => ['card', 'item', 'quiz'],
        'labels' => [
            'requirements' => 'Requirements (Items & Quizzes)',
            'rewards' => 'Rewards (Items & Cards)',
        ]
    ];
    wp_localize_script('nakama-event-ui', 'NakamaUIConfig', $config);
}
