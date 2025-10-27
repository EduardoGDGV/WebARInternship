<?php
if (!defined('ABSPATH')) exit;

add_action('admin_enqueue_scripts', 'nakama_enqueue_admin_ui');

function nakama_enqueue_admin_ui($hook) {
    $screen = get_current_screen();
    if (!$screen || !in_array($screen->post_type, ['event', 'card', 'item', 'quiz', 'asset2d'])) {
        return;
    }

    wp_enqueue_media();

    // Media UI for all post types
    $media_ui_path = plugin_dir_url(__FILE__) . '../js/media-ui.js';
    wp_enqueue_script(
        'nakama-media-ui',
        $media_ui_path,
        ['jquery', 'media-editor', 'media-models', 'media-views'],
        filemtime(plugin_dir_path(__FILE__) . '../js/media-ui.js'),
        true
    );

    switch ($screen->post_type) {
        case 'asset2d':
            return;
        case 'card':
            return;
        case 'item':
            return;
        // Quiz UI only for quiz
        case 'quiz':
            $quiz_ui_path = plugin_dir_url(__FILE__) . '../js/quiz-ui.js';
            wp_enqueue_script(
                'nakama-quiz-ui',
                $quiz_ui_path,
                ['jquery'],
                filemtime(plugin_dir_path(__FILE__) . '../js/quiz-ui.js'),
                true
            );
            return;

        // Event UI only for event
        case 'event':
            $event_ui_path = plugin_dir_url(__FILE__) . '../js/event-ui.js';
            wp_enqueue_script(
                'nakama-event-ui',
                $event_ui_path,
                ['jquery','wp-api-fetch'],
                filemtime(plugin_dir_path(__FILE__) . '../js/event-ui.js'),
                true
            );

            wp_localize_script('nakama-event-ui', 'NakamaUIConfig', [
                'nonce'     => wp_create_nonce('wp_rest'),
                'rest_base' => rest_url('wp/v2'),
                'postTypes' => ['card', 'item', 'quiz'],
                'labels' => [
                    'requirements' => 'Requirements (Items & Quizzes)',
                    'rewards'      => 'Rewards (Items & Cards)'
                ]
            ]);
            return;
    }
}
