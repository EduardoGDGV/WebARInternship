<?php
if ( ! defined( 'ABSPATH' ) ) { exit; }

add_action( 'init', function() {

    // Number meta fields
    $number_meta = [
        'asset_lat'   => 'Latitude',
        'asset_lng'   => 'Longitude',
        'asset_yaw'   => 'Yaw',
        'asset_pitch' => 'Pitch',
        'asset_roll'  => 'Roll',
    ];

    foreach ( $number_meta as $key => $label ) {
        register_post_meta( '', $key, [
            'type' => 'number',
            'single' => true,
            'show_in_rest' => true,
            'auth_callback' => function() { return current_user_can( 'edit_posts' ); },
            'sanitize_callback' => 'floatval',
        ]);
        // Debug
        error_log("Registered number meta: $key ($label)");
    }

    // String meta field
    register_post_meta( '', 'asset_model_url', [
        'type' => 'string',
        'single' => true,
        'show_in_rest' => true,
        'auth_callback' => function() { return current_user_can( 'edit_posts' ); },
        'sanitize_callback' => 'esc_url_raw',
    ]);
    error_log('Registered string meta: asset_model_url');

}, 30);
