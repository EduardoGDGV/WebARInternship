<?php
/**
 * Plugin Name: WP 3D Asset Editor
 * Description: Basic 3D Scene block with Babylon.js canvas rendering.
 * Version: 1.1.0
 * Author: Your Name
 */

defined( 'ABSPATH' ) || exit;

/**
 * Register the block from build/block.json
 */
function wp3d_asset_editor_register_block() {
    error_log( '[WP3D] Registering block from build/block.json' );

    register_block_type( __DIR__ . '/build' );
}
add_action( 'init', 'wp3d_asset_editor_register_block' );

/**
 * Enqueue frontend JS (optional, only if you have frontend logic)
 */
function wp3d_asset_editor_enqueue_frontend() {
    $handle = 'wp3d-frontend';
    $src    = plugins_url( 'frontend.js', __FILE__ );

    // Only enqueue if the file actually exists
    if ( file_exists( plugin_dir_path( __FILE__ ) . 'frontend.js' ) ) {
        error_log( '[WP3D] Enqueuing frontend script: ' . $src );

        wp_enqueue_script(
            $handle,
            $src,
            array(),   // dependencies, e.g., [ 'wp-element' ]
            '1.0',
            true
        );
    } else {
        error_log( '[WP3D] frontend.js not found, skipping enqueue.' );
    }
}
add_action( 'wp_enqueue_scripts', 'wp3d_asset_editor_enqueue_frontend' );
