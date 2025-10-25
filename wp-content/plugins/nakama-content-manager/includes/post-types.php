<?php
if (!defined('ABSPATH')) exit;

add_action('init', function() {

    // EVENT (map anchor)
    register_post_type('event', [
        'label' => 'Events',
        'public' => true,
        'show_in_rest' => true,
        'supports' => ['title', 'thumbnail', 'custom-fields', 'editor'],
    ]);

    register_post_meta('event', 'lat', [
        'type' => 'number',
        'single' => true,
        'show_in_rest' => true,
        'auth_callback' => function() { return current_user_can('edit_posts'); }
    ]);
    register_post_meta('event', 'lon', [
        'type' => 'number',
        'single' => true,
        'show_in_rest' => true,
        'auth_callback' => function() { return current_user_can('edit_posts'); }
    ]);

    // direct image for event marker (attachment ID)
    register_post_meta('event', 'marker_image', [
        'type' => 'integer',
        'single' => true,
        'show_in_rest' => true,
        'auth_callback' => function() { return current_user_can('edit_posts'); }
    ]);

    // Unified relationships (typed arrays)
    register_post_meta('event', 'requirements', [
        'type' => 'array',
        'single' => true,
        'show_in_rest' => [
            'schema' => [
                'type' => 'array',
                'items' => ['type' => 'integer']
            ]
        ],
        'auth_callback' => function() { return current_user_can('edit_posts'); }
    ]);

    register_post_meta('event', 'rewards', [
        'type' => 'array',
        'single' => true,
        'show_in_rest' => [
            'schema' => [
                'type' => 'array',
                'items' => ['type' => 'integer']
            ]
        ],
        'auth_callback' => function() { return current_user_can('edit_posts'); }
    ]);

    // ASSET2D (visual resource only)
    register_post_type('asset2d', [
        'label' => '2D Assets',
        'public' => true,
        'show_in_rest' => true,
        'supports' => ['title', 'thumbnail', 'custom-fields'],
    ]);
    register_post_meta('asset2d', 'image', ['type' => 'integer', 'single' => true, 'show_in_rest' => true]);

    // CARD
    register_post_type('card', [
        'label' => 'Cards',
        'public' => true,
        'show_in_rest' => true,
        'supports' => ['title', 'thumbnail', 'custom-fields'],
    ]);
    register_post_meta('card', 'front_image', ['type' => 'integer', 'single' => true, 'show_in_rest' => true]);
    register_post_meta('card', 'back_image', ['type' => 'integer', 'single' => true, 'show_in_rest' => true]);
    register_post_meta('card', 'is_group', ['type' => 'boolean', 'single' => true, 'show_in_rest' => true, 'default' => false]);

    // ITEM
    register_post_type('item', [
        'label' => 'Items',
        'public' => true,
        'show_in_rest' => true,
        'supports' => ['title', 'thumbnail', 'custom-fields'],
    ]);
    register_post_meta('item', 'image_2d', ['type' => 'integer', 'single' => true, 'show_in_rest' => true]);
    register_post_meta('item', 'image_3d', ['type' => 'string', 'single' => true, 'show_in_rest' => true]);

    // QUIZ
    register_post_type('quiz', [
        'label' => 'Quizzes',
        'public' => true,
        'show_in_rest' => true,
        'supports' => ['title', 'custom-fields'],
    ]);
    register_post_meta('quiz', 'question', ['type' => 'string', 'single' => true, 'show_in_rest' => true]);
    register_post_meta('quiz', 'alternatives', [
        'type' => 'array',
        'single' => true,
        'show_in_rest' => [
            'schema' => ['type' => 'array', 'items' => ['type' => 'string']]
        ]
    ]);
    register_post_meta('quiz', 'answer', ['type' => 'string', 'single' => true, 'show_in_rest' => true]);

});
